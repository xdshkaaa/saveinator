package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
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
	"saveinator/internal/soundcloud"
	"saveinator/internal/spotify"
	"saveinator/internal/tiktok"
	"saveinator/internal/youtube"
)

type Bot struct {
	cfg        *config.Settings
	db         *db.Store
	redis      *redisx.Client
	q          taskEnqueuer
	ytSessions *youtube.SessionStore
	ttSessions *tiktok.SessionStore
	spotify    *spotify.Client
	soundcloud *soundcloud.Client
	fsm        *fsmStore
	runtime    *runtime.Store
}

func New(cfg *config.Settings, store *db.Store, redis *redisx.Client, q taskEnqueuer) *Bot {
	return &Bot{
		cfg:        cfg,
		db:         store,
		redis:      redis,
		q:          q,
		ytSessions: youtube.NewSessionStore(redis.Raw()),
		ttSessions: tiktok.NewSessionStore(redis.Raw()),
		spotify:    spotify.NewClient(cfg.SpotifyClientID, cfg.SpotifyClientSecret, cfg.SpotifyAPITimeoutSeconds),
		soundcloud: soundcloud.NewClient(cfg.SoundCloudTrackTimeoutSeconds, cfg.SoundCloudMaxTracks),
		fsm:        newFSM(),
		runtime:    runtime.NewStore(redis, cfg),
	}
}

func (b *Bot) Register(h *th.BotHandler, bot *telego.Bot) {
	h.Use(metricsMiddleware(b.redis))
	h.HandleMessageCtx(b.onStart(bot), th.CommandEqual("start"))
	h.HandleMessageCtx(b.onSettings(bot), th.CommandEqual("settings"))
	h.HandleMessageCtx(b.onClear(bot), th.CommandEqual("clear"))
	h.HandleMessageCtx(b.onAdmin(bot), th.CommandEqual("admin"))
	h.HandleMessageCtx(b.onStats(bot), th.CommandEqual("stats"))
	h.HandleMessageCtx(b.onBroadcast(bot), th.CommandEqual("broadcast"))
	h.HandleCallbackQueryCtx(b.onLanguageChosen(bot), th.CallbackDataPrefix("lang|"))
	h.HandleCallbackQueryCtx(b.onQualityChoice(bot), th.CallbackDataPrefix("quality:"))
	h.HandleCallbackQueryCtx(b.onYouTubeAction(bot), th.CallbackDataPrefix("yt:"))
	h.HandleCallbackQueryCtx(b.onSettingsCallback(bot), th.CallbackDataPrefix("settings|"))
	h.HandleCallbackQueryCtx(b.onCancelDownload(bot), th.CallbackDataPrefix("dlc:"))
	h.HandleCallbackQueryCtx(b.onDownloadQueue(bot), th.CallbackDataPrefix("dlq:"))
	h.HandleCallbackQueryCtx(b.onAdminCallback(bot), th.CallbackDataPrefix("admin|"))
	h.HandleCallbackQueryCtx(b.onBroadcastCallback(bot), th.CallbackDataPrefix("broadcast|"))
	h.HandleCallbackQueryCtx(b.onTikTokCarousel(bot), th.CallbackDataPrefix("ttk:img:"))
	// Fallback for callback data matching no prefix above (stale/forged buttons).
	// Telego dispatches to the first handler whose predicates match, so this MUST
	// stay registered last or it will shadow every handler above it.
	h.HandleCallbackQueryCtx(b.onUnknownCallback(bot))
	h.HandleMessageCtx(b.onDirectMedia(bot), th.And(
		th.AnyMessage(),
		th.Not(th.Or(th.AnyMessageWithText(), th.AnyMessageWithCaption())),
	))
	h.HandleMessageCtx(b.onText(bot), th.Or(th.AnyMessageWithText(), th.AnyMessageWithCaption()))
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
			_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("onboarding.welcome", lang, nil)))
			return
		}

		kb := tu.InlineKeyboard(
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton(locale.Get("onboarding.btn_en", "en", nil)).WithCallbackData("lang|en"),
				tu.InlineKeyboardButton(locale.Get("onboarding.btn_ru", "en", nil)).WithCallbackData("lang|ru"),
			),
		)
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("onboarding.language_prompt", "en", nil)).WithReplyMarkup(kb))
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
		if lang != "en" && lang != "ru" {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			return
		}

		username := query.From.Username
		firstName := query.From.FirstName
		if err := b.db.CreateUser(ctx, query.From.ID, username, firstName, lang, "saveinator"); err != nil {
			slog.Warn("create user failed", "err", err)
		} else {
			metrics.RecordUserCreated()
		}

		chat := query.Message.GetChat()
		_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
			ChatID:    tu.ID(chat.ID),
			MessageID: query.Message.GetMessageID(),
			Text:      locale.Get("onboarding.welcome", lang, nil),
		})
		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
	}
}

func (b *Bot) onUnknownCallback(bot *telego.Bot) func(context.Context, *telego.Bot, telego.CallbackQuery) {
	return func(ctx context.Context, _ *telego.Bot, query telego.CallbackQuery) {
		lang := "en"
		var userID int64
		if query.From.ID != 0 {
			userID = query.From.ID
			lang = b.userLang(ctx, userID)
		}
		slog.Warn("unmatched callback query", "query_id", query.ID, "user_id", userID, "data", query.Data)
		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).
			WithText(locale.Get("errors.callback_invalid", lang, nil)).
			WithShowAlert())
	}
}

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
		if b.handleYouTubeTrimInput(ctx, bot, msg, lang) {
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
	case linkparser.PlatformSoundCloud:
		if !b.runtime.PlatformEnabled(ctx, "soundcloud") {
			_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("soundcloud.disabled", lang, nil)))
			return
		}
		b.handleSoundCloudLink(ctx, bot, msg, lang, link.URL)
	case linkparser.PlatformPinterest:
		if !b.runtime.PlatformEnabled(ctx, "pinterest") {
			_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("pinterest.disabled", lang, nil)))
			return
		}
		b.enqueueOrReplyError(ctx, bot, msg, lang, link, "pinterest", queue.TypePinterest, batch)
	case linkparser.PlatformTikTok:
		b.enqueueOrReplyError(ctx, bot, msg, lang, link, "tiktok", queue.TypeTikTok, batch)
	case linkparser.PlatformUnknown:
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("errors.unsupported", lang, nil)))
	case linkparser.PlatformYouTube:
		if !b.cfg.YouTubeEnabled {
			_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("errors.unsupported", lang, nil)))
			return
		}
		b.handleYouTubeLink(ctx, bot, msg, lang, link)
	default:
		b.enqueueOrReplyError(ctx, bot, msg, lang, link, string(link.Platform), queue.TypeDownload, batch)
	}
}

func (b *Bot) enqueueOrReplyError(ctx context.Context, bot messageSender, msg telego.Message, lang string, link linkparser.ParsedLink, scene, taskType string, batch bool) {
	if err := b.enqueue(ctx, bot, msg, lang, link, scene, taskType, batch); err != nil {
		slog.Warn("enqueue failed", "platform", link.Platform, "err", err)
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("errors.generic", lang, nil)))
	}
}

func (b *Bot) acquireUserLock(ctx context.Context, userID int64, scene string, ttl time.Duration) (string, bool, error) {
	if b.isAdmin(userID) {
		return "", true, nil
	}
	return b.redis.AcquireUserLock(ctx, userID, scene, ttl)
}

func (b *Bot) enqueue(ctx context.Context, bot messageSender, msg telego.Message, lang string, link linkparser.ParsedLink, scene, taskType string, batch bool) error {
	var token string
	if shouldAcquireUserLock(msg.From.ID, batch, b.cfg.AdminTelegramID) {
		var ok bool
		var err error
		token, ok, err = b.acquireUserLock(ctx, msg.From.ID, scene, lockTTL(b.cfg, scene))
		if err != nil {
			return err
		}
		if !ok {
			b.replyBusy(ctx, bot, msg, lang, scene)
			return nil
		}
	}

	statusMsg := tu.Message(tu.ID(msg.Chat.ID), locale.Get("download.downloading", lang, nil)).WithParseMode(telego.ModeHTML)
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
		Platform:  string(link.Platform),
		ChatID:    msg.Chat.ID,
		UserID:    msg.From.ID,
		MessageID: status.MessageID,
		Lang:      lang,
		LockToken: token,
		LockScene: scene,
		XStatusID: link.XStatusID,
		FormatID:  "best",
	}

	var enqueueErr error
	switch taskType {
	case queue.TypeTikTok:
		enqueueErr = b.q.EnqueueTikTok(payload)
	case queue.TypePinterest:
		enqueueErr = b.q.EnqueuePinterestDefault(payload)
	default:
		enqueueErr = b.q.EnqueueDownload(payload)
	}
	if enqueueErr != nil {
		if token != "" {
			_ = b.redis.ReleaseUserLock(ctx, msg.From.ID, scene, token)
		}
		return fmt.Errorf("enqueue: %w", enqueueErr)
	}
	metrics.DownloadsEnqueued.WithLabelValues(string(link.Platform)).Inc()
	return nil
}

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
	lang, err := b.db.GetUserLanguage(ctx, userID, "saveinator")
	if err != nil || lang == "" {
		return "en"
	}
	return lang
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
	case "soundcloud":
		perTrack := time.Duration(cfg.SoundCloudTrackTimeoutSeconds) * time.Second
		return perTrack*time.Duration(trackCount) + buffer
	default:
		return lockTTL(cfg, scene)
	}
}
