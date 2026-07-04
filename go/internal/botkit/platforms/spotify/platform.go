package spotifyplat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/botkit"
	"saveinator/internal/botkit/botworker"
	"saveinator/internal/cancel"
	"saveinator/internal/config"
	"saveinator/internal/linkparser"
	"saveinator/internal/locale"
	"saveinator/internal/metrics"
	"saveinator/internal/queue"
	"saveinator/internal/spotify"
)

// Platform implements botkit.Platform for Spotify metadata + track downloads.
type Platform struct {
	once   sync.Once
	client *spotify.Client
}

func New() *Platform { return &Platform{} }

func init() {
	botkit.RegisterPlatform("spotify", func() botkit.Platform { return New() })
}

// api lazily builds the Spotify client; config is only available at run time.
func (p *Platform) api(cfg *config.Settings) *spotify.Client {
	p.once.Do(func() {
		p.client = spotify.NewClient(cfg.SpotifyClientID, cfg.SpotifyClientSecret, cfg.SpotifyAPITimeoutSeconds)
	})
	return p.client
}

func (p *Platform) Slug() string { return "spotify" }

func (p *Platform) Match(link linkparser.ParsedLink) bool {
	return link.Platform == linkparser.PlatformSpotify
}

func (p *Platform) HandleLink(ctx context.Context, b *botkit.Bot, tg *telego.Bot, msg telego.Message, lang string, link linkparser.ParsedLink, batch bool) {
	metrics.SpotifyRequestsTotal.Inc()
	if link.SpotifyID == "" || link.SpotifyTyp == "" {
		_, _ = tg.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("errors.unsupported", lang, nil)))
		return
	}

	cfg := b.Cfg()
	token, ok, err := b.AcquireUserLock(ctx, msg.From.ID, "spotify", musicLockTTL(cfg, cfg.SpotifyLockMaxTracks))
	if err != nil {
		slog.Warn("spotify lock failed", "err", err)
		return
	}
	if !ok {
		b.ReplyBusy(ctx, tg, msg, lang, "spotify")
		return
	}

	releaseLock := func() {
		if token == "" {
			return
		}
		_ = b.Redis().ReleaseUserLock(ctx, msg.From.ID, "spotify", token)
	}

	if !cfg.SpotifyEnabled {
		_, _ = tg.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("spotify.disabled", lang, nil)))
		releaseLock()
		return
	}
	if !p.api(cfg).Enabled() {
		_, _ = tg.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("spotify.not_configured", lang, nil)))
		releaseLock()
		return
	}

	release, err := p.api(cfg).FetchRelease(link.SpotifyTyp, link.SpotifyID)
	if err != nil {
		switch err {
		case spotify.ErrNotFound:
			_, _ = tg.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("spotify.not_found", lang, nil)))
		case spotify.ErrAuth:
			_, _ = tg.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("spotify.not_configured", lang, nil)))
		default:
			_, _ = tg.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("spotify.api_error", lang, nil)))
		}
		releaseLock()
		return
	}

	downloadEnabled := b.Runtime().CurrentBool(ctx, "spotify.download_enabled", cfg.SpotifyDownloadEnabled)
	text := spotify.CardText(release, lang, downloadEnabled)
	kb := spotify.OpenKeyboard(release, lang)
	sendMusicCard(tg, msg.Chat.ID, release.CoverURL, text, kb)

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
	status, err := tg.SendMessage(statusMsg)
	if err != nil {
		releaseLock()
		return
	}

	if err := b.Queue().EnqueueMusicTo(queue.TypeSpotify, b.BotCfg().Queue, queue.MusicPayload{
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
		_, _ = tg.EditMessageText(&telego.EditMessageTextParams{
			ChatID:    tu.ID(msg.Chat.ID),
			MessageID: status.MessageID,
			Text:      locale.Get("spotify.download_failed", lang, nil),
		})
		releaseLock()
		return
	}
	metrics.DownloadsEnqueued.WithLabelValues("spotify").Inc()
}

func sendMusicCard(tg *telego.Bot, chatID int64, coverURL, text string, kb *telego.InlineKeyboardMarkup) {
	if coverURL != "" {
		_, _ = tg.SendPhoto(&telego.SendPhotoParams{
			ChatID:      tu.ID(chatID),
			Photo:       tu.FileFromURL(coverURL),
			Caption:     text,
			ReplyMarkup: kb,
		})
		return
	}
	_, _ = tg.SendMessage(tu.Message(tu.ID(chatID), text).WithReplyMarkup(kb))
}

func musicLockTTL(cfg *config.Settings, trackCount int) time.Duration {
	if trackCount < 1 {
		trackCount = 1
	}
	perTrack := time.Duration(cfg.SpotifyTrackTimeoutSeconds) * time.Second
	return perTrack*time.Duration(trackCount) + 90*time.Second
}

func (p *Platform) RegisterWorker(mux *asynq.ServeMux, d *botworker.Deps, bc *botkit.BotConfig) {
	h := &taskHandler{d: d, botID: bc.Slug}
	mux.HandleFunc(queue.TypeSpotify, h.handle)
}
