package botkit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/cancel"
	"saveinator/internal/config"
	"saveinator/internal/db"
	"saveinator/internal/linkparser"
	"saveinator/internal/locale"
	"saveinator/internal/metrics"
	"saveinator/internal/queue"
	"saveinator/internal/redisx"
	"saveinator/internal/runtime"
)

type Bot struct {
	bc      *BotConfig
	cfg     *config.Settings
	db      *db.Store
	redis   *redisx.Client
	q       *queue.Client
	runtime *runtime.Store
	fsm     *fsmStore
}

func New(bc *BotConfig, cfg *config.Settings, store *db.Store, redis *redisx.Client, q *queue.Client) *Bot {
	return &Bot{
		bc:      bc,
		cfg:     cfg,
		db:      store,
		redis:   redis,
		q:       q,
		runtime: runtime.NewStore(redis, cfg),
		fsm:     newFSM(),
	}
}

// Accessors for Platform implementations.

func (b *Bot) BotCfg() *BotConfig      { return b.bc }
func (b *Bot) Cfg() *config.Settings   { return b.cfg }
func (b *Bot) DB() *db.Store           { return b.db }
func (b *Bot) Redis() *redisx.Client   { return b.redis }
func (b *Bot) Queue() *queue.Client    { return b.q }
func (b *Bot) Runtime() *runtime.Store { return b.runtime }

func (b *Bot) Register(h *th.BotHandler, bot *telego.Bot) {
	h.Use(metricsMiddleware(b.redis, b.bc.Slug))
	h.HandleMessageCtx(b.onStart(bot), th.CommandEqual("start"))
	h.HandleMessageCtx(b.onLang(bot), th.CommandEqual("lang"))
	h.HandleMessageCtx(b.onSettings(bot), th.CommandEqual("settings"))
	h.HandleMessageCtx(b.onClear(bot), th.CommandEqual("clear"))
	h.HandleMessageCtx(b.onAdmin(bot), th.CommandEqual("admin"))
	h.HandleMessageCtx(b.onStats(bot), th.CommandEqual("stats"))
	h.HandleMessageCtx(b.onBroadcast(bot), th.CommandEqual("broadcast"))
	h.HandleCallbackQueryCtx(b.onLanguageChosen(bot), th.CallbackDataPrefix("lang|"))
	h.HandleCallbackQueryCtx(b.onSettingsCallback(bot), th.CallbackDataPrefix("settings|"))
	h.HandleCallbackQueryCtx(b.onCancelDownload(bot), th.CallbackDataPrefix("dlc:"))
	h.HandleCallbackQueryCtx(b.onDownloadQueue(bot), th.CallbackDataPrefix("dlq:"))
	h.HandleCallbackQueryCtx(b.onAdminCallback(bot), th.CallbackDataPrefix("admin|"))
	h.HandleCallbackQueryCtx(b.onBroadcastCallback(bot), th.CallbackDataPrefix("broadcast|"))
	h.HandleMessageCtx(b.onDirectMedia(bot), th.And(
		th.AnyMessage(),
		th.Not(th.Or(th.AnyMessageWithText(), th.AnyMessageWithCaption())),
	))
	h.HandleMessageCtx(b.onText(bot), th.Or(th.AnyMessageWithText(), th.AnyMessageWithCaption()))
}

// ---- onStart / onboarding ----

func (b *Bot) languageKeyboard(callbackPrefix string) *telego.InlineKeyboardMarkup {
	var buttons []telego.InlineKeyboardButton
	for _, code := range b.bc.Languages {
		buttons = append(buttons, tu.InlineKeyboardButton(languageButtonLabel(code)).
			WithCallbackData(callbackPrefix+code))
	}
	return tu.InlineKeyboard(tu.InlineKeyboardRow(buttons...))
}

func languageButtonLabel(code string) string {
	return locale.SelfName(code)
}

func (b *Bot) onStart(bot *telego.Bot) func(context.Context, *telego.Bot, telego.Message) {
	return func(ctx context.Context, _ *telego.Bot, msg telego.Message) {
		if msg.From == nil {
			return
		}
		metrics.RecordCommand("start")
		lang := b.userLang(ctx, msg.From.ID)
		exists, err := b.db.UserExists(ctx, msg.From.ID)
		if err != nil {
			slog.Warn("user lookup failed", "err", err)
		}
		if exists {
			_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get(b.bc.WelcomeKey, lang, nil)))
			return
		}

		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("onboarding.language_prompt", "en", nil)).WithReplyMarkup(b.languageKeyboard("lang|")))
	}
}

func (b *Bot) onLang(bot *telego.Bot) func(context.Context, *telego.Bot, telego.Message) {
	return func(ctx context.Context, _ *telego.Bot, msg telego.Message) {
		if msg.From == nil {
			return
		}
		metrics.RecordCommand("lang")
		lang := b.userLang(ctx, msg.From.ID)
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("onboarding.lang_command_prompt", lang, nil)).WithReplyMarkup(b.languageKeyboard("lang|")))
	}
}

func (b *Bot) onLanguageChosen(bot *telego.Bot) func(context.Context, *telego.Bot, telego.CallbackQuery) {
	return func(ctx context.Context, _ *telego.Bot, query telego.CallbackQuery) {
		if query.From.ID == 0 || query.Message == nil {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			return
		}
		parts := strings.Split(query.Data, "|")
		if len(parts) != 2 {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			return
		}
		lang := parts[1]
		if !b.bc.langAllowed(lang) {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			return
		}

		exists, err := b.db.UserExists(ctx, query.From.ID)
		if err != nil {
			slog.Warn("user lookup failed", "err", err)
		}
		if !exists {
			username := query.From.Username
			firstName := query.From.FirstName
			if err := b.db.CreateUser(ctx, query.From.ID, username, firstName, lang, b.bc.Slug); err != nil {
				slog.Warn("create user failed", "err", err)
			} else {
				metrics.RecordUserCreated()
			}
		}
		if err := b.db.SetUserLanguage(ctx, query.From.ID, lang, b.bc.Slug); err != nil {
			slog.Warn("set user language failed", "err", err)
		}

		chat := query.Message.GetChat()
		_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
			ChatID:    tu.ID(chat.ID),
			MessageID: query.Message.GetMessageID(),
			Text:      locale.Get(b.bc.WelcomeKey, lang, nil),
		})
		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText(locale.Get("onboarding.lang_changed", lang, nil)))
	}
}

// ---- Direct media rejection ----

func (b *Bot) onDirectMedia(bot *telego.Bot) func(context.Context, *telego.Bot, telego.Message) {
	return func(ctx context.Context, _ *telego.Bot, msg telego.Message) {
		if msg.From == nil || !hasAttachedMedia(msg) {
			return
		}
		if strings.HasPrefix(msg.Caption, "/") {
			return
		}
		lang := b.userLang(ctx, msg.From.ID)
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("errors.send_link", lang, nil)))
	}
}

func hasAttachedMedia(msg telego.Message) bool {
	return msg.Video != nil || msg.Document != nil || len(msg.Photo) > 0 || msg.Animation != nil
}

// ---- Message flow ----

func (b *Bot) onText(bot *telego.Bot) func(context.Context, *telego.Bot, telego.Message) {
	return func(ctx context.Context, _ *telego.Bot, msg telego.Message) {
		if shouldSkipIncomingMessage(msg) {
			return
		}

		lang := b.userLang(ctx, msg.From.ID)
		if b.checkBanned(ctx, bot, msg, lang) {
			return
		}
		if !b.allowGroupLinks(ctx, msg) {
			return
		}
		if b.handleAdminFSM(ctx, bot, msg, lang) {
			return
		}

		if !b.allowRateLimit(ctx, bot, msg, lang) {
			return
		}

		links := extractMessageLinks(msg)
		if len(links) == 0 {
			return
		}
		for _, link := range links {
			b.dispatchLink(ctx, bot, msg, lang, link, len(links) > 1)
		}
	}
}

func (b *Bot) dispatchLink(ctx context.Context, bot *telego.Bot, msg telego.Message, lang string, link linkparser.ParsedLink, batch bool) {
	for _, p := range b.bc.Platforms {
		if !p.Match(link) {
			continue
		}
		if !b.runtime.PlatformEnabled(ctx, p.Slug()) {
			_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get(p.Slug()+".disabled", lang, nil)))
			return
		}
		p.HandleLink(ctx, b, bot, msg, lang, link, batch)
		return
	}
	_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get(b.bc.NotSupportedKey, lang, nil)))
}

// EnqueueDownload runs the generic status-message + lock + enqueue flow used
// by download platforms (Pinterest and friends).
func (b *Bot) EnqueueDownload(ctx context.Context, bot *telego.Bot, msg telego.Message, lang string, link linkparser.ParsedLink, scene, taskType string, batch bool) {
	if err := b.enqueue(ctx, bot, msg, lang, link, scene, taskType, batch); err != nil {
		slog.Warn("enqueue failed", "platform", link.Platform, "err", err)
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("errors.generic", lang, nil)))
	}
}

func (b *Bot) enqueue(ctx context.Context, bot messageSender, msg telego.Message, lang string, link linkparser.ParsedLink, scene, taskType string, batch bool) error {
	var token string
	if shouldAcquireUserLock(msg.From.ID, batch, b.cfg.AdminTelegramID) {
		var ok bool
		var err error
		token, ok, err = b.AcquireUserLock(ctx, msg.From.ID, scene, lockTTL(b.cfg, scene))
		if err != nil {
			return err
		}
		if !ok {
			b.ReplyBusy(ctx, bot, msg, lang, scene)
			return nil
		}
	}

	statusMsg := tu.Message(tu.ID(msg.Chat.ID), locale.Get("download.downloading", lang, nil))
	if token != "" {
		statusMsg = statusMsg.WithReplyMarkup(cancel.Keyboard(lang, scene, msg.From.ID, token))
	}
	status, err := bot.SendMessage(statusMsg)
	if err != nil {
		if token != "" {
			_ = b.redis.ReleaseUserLock(ctx, msg.From.ID, scene, token)
		}
		return err
	}

	payload := queue.DownloadPayload{
		URL:       link.URL,
		Platform:  scene,
		ChatID:    msg.Chat.ID,
		UserID:    msg.From.ID,
		MessageID: status.MessageID,
		Lang:      lang,
		LockToken: token,
		LockScene: scene,
	}

	if enqueueErr := b.q.EnqueueDownloadTo(taskType, b.bc.Queue, payload); enqueueErr != nil {
		if token != "" {
			_ = b.redis.ReleaseUserLock(ctx, msg.From.ID, scene, token)
		}
		return fmt.Errorf("enqueue: %w", enqueueErr)
	}
	metrics.DownloadsEnqueued.WithLabelValues(scene).Inc()
	metrics.BotDownloadsEnqueuedTotal.WithLabelValues(b.bc.Slug, scene).Inc()
	return nil
}

func (b *Bot) ReplyBusy(_ context.Context, bot messageSender, msg telego.Message, lang, scenario string) {
	metrics.RecordUserQueueRejected(scenario)
	kb := cancel.QueueButton(lang, msg.From.ID)
	_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("errors.busy", lang, nil)).WithReplyMarkup(kb))
}

// ---- Lock helpers ----

func (b *Bot) AcquireUserLock(ctx context.Context, userID int64, scene string, ttl time.Duration) (string, bool, error) {
	if b.isAdmin(userID) {
		return "", true, nil
	}
	return b.redis.AcquireUserLock(ctx, userID, scene, ttl)
}

func lockTTL(cfg *config.Settings, scene string) time.Duration {
	base := time.Duration(cfg.DownloadTimeoutSeconds) * time.Second
	if base < time.Minute {
		base = time.Minute
	}
	_ = scene
	return base + 30*time.Second
}

// ---- Rate limit ----

func (b *Bot) allowRateLimit(ctx context.Context, bot messageSender, msg telego.Message, lang string) bool {
	if msg.From != nil && b.isAdmin(msg.From.ID) {
		return true
	}
	window := time.Minute
	if msg.From != nil {
		ok, err := b.redis.AllowRateLimit(ctx, "user", msg.From.ID, b.cfg.RateLimitUserPerMinute, window)
		if err != nil {
			slog.Warn("rate limit check failed", "err", err)
			return true
		}
		if !ok {
			metrics.RateLimitDropped.WithLabelValues("user").Inc()
			if msg.Chat.Type == "private" {
				_, _ = bot.SendMessage(tu.Message(
					tu.ID(msg.Chat.ID),
					locale.Get("errors.rate_limit", lang, map[string]string{
						"count":  fmt.Sprintf("%d", b.cfg.RateLimitUserPerMinute),
						"window": "60",
					}),
				))
			}
			return false
		}
	}
	ok, err := b.redis.AllowRateLimit(ctx, "chat", msg.Chat.ID, b.cfg.RateLimitChatPerMinute, window)
	if err != nil {
		return true
	}
	if !ok {
		metrics.RateLimitDropped.WithLabelValues("chat").Inc()
		if msg.Chat.Type != "private" {
			_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("errors.chat_rate_limit", lang, nil)))
		}
		return false
	}
	return true
}

func (b *Bot) userLang(ctx context.Context, userID int64) string {
	lang, err := b.db.GetUserLanguage(ctx, userID, b.bc.Slug)
	if err != nil || lang == "" {
		return b.bc.DefaultLang
	}
	return lang
}

// UserLang exposes the stored (or default) language to platforms.
func (b *Bot) UserLang(ctx context.Context, userID int64) string {
	return b.userLang(ctx, userID)
}

// ---- Ban helpers ----

func (b *Bot) isAdmin(userID int64) bool {
	return b.cfg.AdminTelegramID != 0 && userID == b.cfg.AdminTelegramID
}

// ---- FSM ----

type pendingState struct {
	Kind string
	Data map[string]string
}

type fsmStore struct {
	mu     sync.Mutex
	states map[int64]*pendingState
}

func newFSM() *fsmStore {
	return &fsmStore{states: make(map[int64]*pendingState)}
}

func (f *fsmStore) Set(userID int64, kind string, data map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states[userID] = &pendingState{Kind: kind, Data: data}
}

func (f *fsmStore) Get(userID int64) (*pendingState, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.states[userID]
	return s, ok
}

func (f *fsmStore) Clear(userID int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.states, userID)
}

// ---- Spam / group link dedup ----

func (b *Bot) allowGroupLinks(ctx context.Context, msg telego.Message) bool {
	body := messageBody(msg)
	if msg.Chat.Type == "private" || body == "" {
		return true
	}

	links := linkparser.ExtractURLs(body)
	if len(links) == 0 {
		return true
	}

	window := time.Duration(b.cfg.SpamDedupWindowSeconds) * time.Second
	if window <= 0 {
		window = 5 * time.Minute
	}

	for _, link := range links {
		sum := sha256.Sum256([]byte(link.URL))
		urlHash := hex.EncodeToString(sum[:])

		banned, err := b.db.IsLinkBanned(ctx, urlHash)
		if err != nil {
			slog.Warn("banned link check failed", "err", err)
		} else if banned {
			metrics.SpamBlocked.WithLabelValues("banned").Inc()
			return false
		}

		ok, err := b.redis.AllowURLDedup(ctx, urlHash, window)
		if err != nil {
			slog.Warn("url dedup check failed", "err", err)
			continue
		}
		if !ok {
			metrics.SpamBlocked.WithLabelValues("dedup").Inc()
			return false
		}
	}
	return true
}
