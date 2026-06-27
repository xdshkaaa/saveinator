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
	q          *queue.Client
	ytSessions *youtube.SessionStore
	ttSessions *tiktok.SessionStore
	spotify    *spotify.Client
	soundcloud *soundcloud.Client
	fsm        *fsmStore
	runtime    *runtime.Store
}

func New(cfg *config.Settings, store *db.Store, redis *redisx.Client, q *queue.Client) *Bot {
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
	h.HandleMessageCtx(b.onStart(bot), th.CommandEqual("start"))
	h.HandleMessageCtx(b.onSettings(bot), th.CommandEqual("settings"))
	h.HandleMessageCtx(b.onAdmin(bot), th.CommandEqual("admin"))
	h.HandleMessageCtx(b.onStats(bot), th.CommandEqual("stats"))
	h.HandleMessageCtx(b.onBroadcast(bot), th.CommandEqual("broadcast"))
	h.HandleCallbackQueryCtx(b.onLanguageChosen(bot), th.CallbackDataPrefix("lang|"))
	h.HandleCallbackQueryCtx(b.onQualityChoice(bot), th.CallbackDataPrefix("quality:"))
	h.HandleCallbackQueryCtx(b.onRatioChoice(bot), th.CallbackDataPrefix("ratio:"))
	h.HandleCallbackQueryCtx(b.onSettingsCallback(bot), th.CallbackDataPrefix("settings|"))
	h.HandleCallbackQueryCtx(b.onCancelDownload(bot), th.CallbackDataPrefix("dlc:"))
	h.HandleCallbackQueryCtx(b.onDownloadQueue(bot), th.CallbackDataPrefix("dlq:"))
	h.HandleCallbackQueryCtx(b.onAdminCallback(bot), th.CallbackDataPrefix("admin|"))
	h.HandleCallbackQueryCtx(b.onAdminBroadcasts(bot), th.CallbackDataEqual("admin|broadcasts"))
	h.HandleCallbackQueryCtx(b.onBroadcastCallback(bot), th.CallbackDataPrefix("broadcast|"))
	h.HandleCallbackQueryCtx(b.onTikTokCarousel(bot), th.CallbackDataPrefix("ttk:img:"))
	h.HandleMessageCtx(b.onText(bot), th.AnyMessage())
}

func (b *Bot) onStart(bot *telego.Bot) func(context.Context, *telego.Bot, telego.Message) {
	return func(ctx context.Context, _ *telego.Bot, msg telego.Message) {
		if msg.From == nil {
			return
		}
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
		if err := b.db.CreateUser(ctx, query.From.ID, username, firstName, lang); err != nil {
			slog.Warn("create user failed", "err", err)
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

func (b *Bot) onText(bot *telego.Bot) func(context.Context, *telego.Bot, telego.Message) {
	return func(ctx context.Context, _ *telego.Bot, msg telego.Message) {
		if msg.Text == "" || msg.From == nil {
			return
		}
		if strings.HasPrefix(msg.Text, "/") {
			return
		}

		lang := b.userLang(ctx, msg.From.ID)
		if b.checkBanned(ctx, bot, msg, lang) {
			return
		}
		if b.handleAdminFSM(ctx, bot, msg, lang) {
			return
		}

		if !b.allowRateLimit(ctx, bot, msg, lang) {
			return
		}

		links := linkparser.ExtractURLs(msg.Text)
		if len(links) == 0 {
			return
		}
		link := links[0]

		switch link.Platform {
		case linkparser.PlatformSpotify:
			b.handleSpotifyLink(ctx, bot, msg, lang, link)
		case linkparser.PlatformSoundCloud:
			b.handleSoundCloudLink(ctx, bot, msg, lang, link.URL)
		case linkparser.PlatformPinterest:
			if !b.cfg.PinterestEnabled {
				_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("pinterest.disabled", lang, nil)))
				return
			}
			_ = b.enqueue(ctx, bot, msg, lang, link, "pinterest", queue.TypePinterest)
		case linkparser.PlatformTikTok:
			_ = b.enqueue(ctx, bot, msg, lang, link, "tiktok", queue.TypeTikTok)
		case linkparser.PlatformUnknown:
			_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("errors.unsupported", lang, nil)))
		case linkparser.PlatformYouTube:
			b.handleYouTubeLink(ctx, bot, msg, lang, link)
		default:
			_ = b.enqueue(ctx, bot, msg, lang, link, string(link.Platform), queue.TypeDownload)
		}
	}
}

func (b *Bot) enqueue(ctx context.Context, bot *telego.Bot, msg telego.Message, lang string, link linkparser.ParsedLink, scene, taskType string) error {
	token, ok, err := b.redis.AcquireUserLock(ctx, msg.From.ID, scene, lockTTL(b.cfg, scene))
	if err != nil {
		return err
	}
	if !ok {
		b.replyBusy(ctx, bot, msg, lang)
		return nil
	}

	status, err := bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("download.downloading", lang, nil)).WithReplyMarkup(
		cancel.Keyboard(lang, scene, msg.From.ID, token),
	))
	if err != nil {
		_ = b.redis.ReleaseUserLock(ctx, msg.From.ID, scene, token)
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
		enqueueErr = b.q.EnqueuePinterest(payload)
	default:
		enqueueErr = b.q.EnqueueDownload(payload)
	}
	if enqueueErr != nil {
		_ = b.redis.ReleaseUserLock(ctx, msg.From.ID, scene, token)
		return fmt.Errorf("enqueue: %w", enqueueErr)
	}
	return nil
}

func (b *Bot) allowRateLimit(ctx context.Context, bot *telego.Bot, msg telego.Message, lang string) bool {
	if msg.From != nil && msg.From.ID == b.cfg.AdminTelegramID {
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
	return ok
}

func (b *Bot) userLang(ctx context.Context, userID int64) string {
	lang, err := b.db.GetUserLanguage(ctx, userID)
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
