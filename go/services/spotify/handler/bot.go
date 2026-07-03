package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	"saveinator/internal/spotify"
)

type Bot struct {
	cfg     *config.Settings
	db      *db.Store
	redis   *redisx.Client
	q       taskEnqueuer
	runtime *runtime.Store
	fsm     *fsmStore
	spotify *spotify.Client
}

func New(cfg *config.Settings, store *db.Store, redis *redisx.Client, q taskEnqueuer) *Bot {
	return &Bot{
		cfg:     cfg,
		db:      store,
		redis:   redis,
		q:       q,
		runtime: runtime.NewStore(redis, cfg),
		fsm:     newFSM(),
		spotify: spotify.NewClient(cfg.SpotifyClientID, cfg.SpotifyClientSecret, cfg.SpotifyAPITimeoutSeconds),
	}
}

func (b *Bot) Register(h *th.BotHandler, bot *telego.Bot) {
	h.Use(metricsMiddleware(b.redis))
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

func (b *Bot) languageKeyboard() *telego.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(locale.Get("onboarding.btn_en", "en", nil)).WithCallbackData("lang|en"),
			tu.InlineKeyboardButton(locale.Get("onboarding.btn_ru", "en", nil)).WithCallbackData("lang|ru"),
			tu.InlineKeyboardButton(locale.Get("onboarding.btn_kk", "en", nil)).WithCallbackData("lang|kk"),
		),
	)
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
			_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("onboarding.welcome_spotify", lang, nil)))
			return
		}

		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("onboarding.language_prompt", "en", nil)).WithReplyMarkup(b.languageKeyboard()))
	}
}

func (b *Bot) onLang(bot *telego.Bot) func(context.Context, *telego.Bot, telego.Message) {
	return func(ctx context.Context, _ *telego.Bot, msg telego.Message) {
		if msg.From == nil {
			return
		}
		metrics.RecordCommand("lang")
		lang := b.userLang(ctx, msg.From.ID)
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("onboarding.lang_command_prompt", lang, nil)).WithReplyMarkup(b.languageKeyboard()))
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
		if lang != "en" && lang != "ru" && lang != "kk" {
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
			if err := b.db.CreateUser(ctx, query.From.ID, username, firstName, lang); err != nil {
				slog.Warn("create user failed", "err", err)
			} else {
				metrics.RecordUserCreated()
			}
		}
		if err := b.db.SetUserLanguage(ctx, query.From.ID, lang); err != nil {
			slog.Warn("set user language failed", "err", err)
		}

		chat := query.Message.GetChat()
		_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
			ChatID:    tu.ID(chat.ID),
			MessageID: query.Message.GetMessageID(),
			Text:      locale.Get("onboarding.welcome_spotify", lang, nil),
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
	switch link.Platform {
	case linkparser.PlatformSpotify:
		if !b.runtime.PlatformEnabled(ctx, "spotify") {
			_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("spotify.disabled", lang, nil)))
			return
		}
		b.handleSpotifyLink(ctx, bot, msg, lang, link)
	default:
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("spotify.not_spotify_link", lang, nil)))
	}
}

func (b *Bot) handleSpotifyLink(ctx context.Context, bot *telego.Bot, msg telego.Message, lang string, link linkparser.ParsedLink) {
	metrics.SpotifyRequestsTotal.Inc()
	if link.SpotifyID == "" || link.SpotifyTyp == "" {
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("errors.unsupported", lang, nil)))
		return
	}

	token, ok, err := b.acquireUserLock(ctx, msg.From.ID, "spotify", musicLockTTL(b.cfg, "spotify", b.cfg.SpotifyLockMaxTracks))
	if err != nil {
		slog.Warn("spotify lock failed", "err", err)
		return
	}
	if !ok {
		b.replyBusy(ctx, bot, msg, lang, "spotify")
		return
	}

	releaseLock := func() {
		if token == "" {
			return
		}
		_ = b.redis.ReleaseUserLock(ctx, msg.From.ID, "spotify", token)
	}

	if !b.cfg.SpotifyEnabled {
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("spotify.disabled", lang, nil)))
		releaseLock()
		return
	}
	if !b.spotify.Enabled() {
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("spotify.not_configured", lang, nil)))
		releaseLock()
		return
	}

	release, err := b.spotify.FetchRelease(link.SpotifyTyp, link.SpotifyID)
	if err != nil {
		switch err {
		case spotify.ErrNotFound:
			_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("spotify.not_found", lang, nil)))
		case spotify.ErrAuth:
			_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("spotify.not_configured", lang, nil)))
		default:
			_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("spotify.api_error", lang, nil)))
		}
		releaseLock()
		return
	}

	downloadEnabled := b.runtime.CurrentBool(ctx, "spotify.download_enabled", b.cfg.SpotifyDownloadEnabled)
	text := spotify.CardText(release, lang, downloadEnabled)
	kb := spotify.OpenKeyboard(release, lang)
	b.sendMusicCard(bot, msg.Chat.ID, release.CoverURL, text, kb)

	if !downloadEnabled || len(release.Tracks) == 0 {
		releaseLock()
		return
	}

	releaseJSON, err := json.Marshal(release)
	if err != nil {
		releaseLock()
		return
	}

	statusText := locale.Get("spotify.download_starting", lang, map[string]string{
		"total": fmt.Sprintf("%d", len(release.Tracks)),
	})
	statusMsg := tu.Message(tu.ID(msg.Chat.ID), statusText)
	if token != "" {
		statusMsg = statusMsg.WithReplyMarkup(cancel.Keyboard(lang, "spotify", msg.From.ID, token))
	}
	status, err := bot.SendMessage(statusMsg)
	if err != nil {
		releaseLock()
		return
	}

	if err := b.q.EnqueueSpotifyMicroservice(queue.MusicPayload{
		Platform:    "spotify",
		ChatID:      msg.Chat.ID,
		UserID:      msg.From.ID,
		MessageID:   status.MessageID,
		Lang:        lang,
		LockToken:   token,
		LockScene:   "spotify",
		LinkType:    link.SpotifyTyp,
		ResourceID:  link.SpotifyID,
		ReleaseJSON: string(releaseJSON),
	}); err != nil {
		slog.Warn("enqueue spotify failed", "err", err)
		_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
			ChatID:    tu.ID(msg.Chat.ID),
			MessageID: status.MessageID,
			Text:      locale.Get("spotify.download_failed", lang, nil),
		})
		releaseLock()
		return
	}
	metrics.DownloadsEnqueued.WithLabelValues("spotify").Inc()
}

func (b *Bot) sendMusicCard(bot *telego.Bot, chatID int64, coverURL, text string, kb *telego.InlineKeyboardMarkup) {
	if coverURL != "" {
		_, _ = bot.SendPhoto(&telego.SendPhotoParams{
			ChatID:      tu.ID(chatID),
			Photo:       tu.FileFromURL(coverURL),
			Caption:     text,
			ReplyMarkup: kb,
		})
		return
	}
	_, _ = bot.SendMessage(tu.Message(tu.ID(chatID), text).WithReplyMarkup(kb))
}

func (b *Bot) replyBusy(_ context.Context, bot messageSender, msg telego.Message, lang, scenario string) {
	metrics.RecordUserQueueRejected(scenario)
	kb := cancel.QueueButton(lang, msg.From.ID)
	_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("errors.busy", lang, nil)).WithReplyMarkup(kb))
}

// ---- Lock helpers ----

func (b *Bot) acquireUserLock(ctx context.Context, userID int64, scene string, ttl time.Duration) (string, bool, error) {
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

func musicLockTTL(cfg *config.Settings, scene string, trackCount int) time.Duration {
	if trackCount < 1 {
		trackCount = 1
	}
	buffer := 90 * time.Second
	switch scene {
	case "spotify":
		perTrack := time.Duration(cfg.SpotifyTrackTimeoutSeconds) * time.Second
		return perTrack*time.Duration(trackCount) + buffer
	default:
		return lockTTL(cfg, scene)
	}
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
	lang, err := b.db.GetUserLanguage(ctx, userID)
	if err != nil || lang == "" {
		return "en"
	}
	return lang
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
