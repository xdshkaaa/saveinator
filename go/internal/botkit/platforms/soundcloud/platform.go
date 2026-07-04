package soundcloudplat

import (
	"context"
	"encoding/json"
	"errors"
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
	"saveinator/internal/soundcloud"
)

// Platform implements botkit.Platform for SoundCloud track/playlist downloads.
type Platform struct {
	once   sync.Once
	client *soundcloud.Client
}

func New() *Platform { return &Platform{} }

func init() {
	botkit.RegisterPlatform("soundcloud", func() botkit.Platform { return New() })
}

// api lazily builds the SoundCloud client; config is only available at run time.
func (p *Platform) api(cfg *config.Settings) *soundcloud.Client {
	p.once.Do(func() {
		p.client = soundcloud.NewClient(cfg.SoundCloudTrackTimeoutSeconds, cfg.SoundCloudMaxTracks)
	})
	return p.client
}

func (p *Platform) Slug() string { return "soundcloud" }

func (p *Platform) Match(link linkparser.ParsedLink) bool {
	return link.Platform == linkparser.PlatformSoundCloud
}

func (p *Platform) HandleLink(ctx context.Context, b *botkit.Bot, tg *telego.Bot, msg telego.Message, lang string, link linkparser.ParsedLink, batch bool) {
	metrics.SoundCloudRequestsTotal.Inc()
	scLink, err := soundcloud.ParseLink(link.URL)
	if err != nil {
		_, _ = tg.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("errors.unsupported", lang, nil)))
		return
	}

	cfg := b.Cfg()
	token, ok, err := b.AcquireUserLock(ctx, msg.From.ID, "soundcloud", musicLockTTL(cfg, cfg.SoundCloudMaxTracks))
	if err != nil {
		slog.Warn("soundcloud lock failed", "err", err)
		return
	}
	if !ok {
		b.ReplyBusy(ctx, tg, msg, lang, "soundcloud")
		return
	}

	releaseLock := func() {
		if token == "" {
			return
		}
		_ = b.Redis().ReleaseUserLock(ctx, msg.From.ID, "soundcloud", token)
	}

	if !cfg.SoundCloudEnabled {
		_, _ = tg.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("soundcloud.disabled", lang, nil)))
		releaseLock()
		return
	}

	maxTracks := b.Runtime().CurrentInt(ctx, "soundcloud.max_tracks_per_playlist", cfg.SoundCloudMaxTracks)
	release, err := p.api(cfg).FetchRelease(ctx, scLink, maxTracks)
	if err != nil {
		switch {
		case errors.Is(err, soundcloud.ErrNotFound):
			_, _ = tg.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("soundcloud.not_found", lang, nil)))
		case errors.Is(err, soundcloud.ErrTooLarge):
			_, _ = tg.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("soundcloud.playlist_too_large", lang, map[string]string{
				"limit": fmt.Sprintf("%d", maxTracks),
			})))
		default:
			metrics.SoundCloudMetadataFailuresTotal.Inc()
			_, _ = tg.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("soundcloud.download_failed", lang, nil)))
		}
		releaseLock()
		return
	}

	downloadEnabled := b.Runtime().CurrentBool(ctx, "soundcloud.download_enabled", cfg.SoundCloudDownloadEnabled)
	text := soundcloud.CardText(release, lang, downloadEnabled)
	kb := soundcloud.OpenKeyboard(release, lang)
	sendMusicCard(tg, msg.Chat.ID, release.ArtworkURL, text, kb)

	if !downloadEnabled || len(release.Tracks) == 0 {
		releaseLock()
		return
	}
	if scLink.Type == soundcloud.LinkTypePlaylist {
		metrics.SoundCloudPlaylistTracks.Observe(float64(len(release.Tracks)))
	}

	releaseJSON, err := json.Marshal(release)
	if err != nil {
		releaseLock()
		return
	}

	statusText := locale.Get("soundcloud.download_starting", lang, map[string]string{
		"total": fmt.Sprintf("%d", len(release.Tracks)),
	})
	statusMsg := tu.Message(tu.ID(msg.Chat.ID), statusText)
	if token != "" {
		statusMsg = statusMsg.WithReplyMarkup(cancel.Keyboard(lang, "soundcloud", msg.From.ID, token))
	}
	status, err := tg.SendMessage(statusMsg)
	if err != nil {
		releaseLock()
		return
	}

	if err := b.Queue().EnqueueMusicTo(queue.TypeSoundCloud, b.BotCfg().Queue, queue.MusicPayload{
		Platform:    "soundcloud",
		ChatID:      msg.Chat.ID,
		UserID:      msg.From.ID,
		MessageID:   status.MessageID,
		Lang:        lang,
		LockToken:   token,
		LockScene:   "soundcloud",
		SourceURL:   scLink.URL,
		ReleaseJSON: string(releaseJSON),
	}); err != nil {
		slog.Warn("enqueue soundcloud failed", "err", err)
		_, _ = tg.EditMessageText(&telego.EditMessageTextParams{
			ChatID:    tu.ID(msg.Chat.ID),
			MessageID: status.MessageID,
			Text:      locale.Get("soundcloud.download_failed", lang, nil),
		})
		releaseLock()
		return
	}
	metrics.DownloadsEnqueued.WithLabelValues("soundcloud").Inc()
	metrics.SoundCloudDownloadsEnqueuedTotal.Inc()
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
	perTrack := time.Duration(cfg.SoundCloudTrackTimeoutSeconds) * time.Second
	return perTrack*time.Duration(trackCount) + 90*time.Second
}

func (p *Platform) RegisterWorker(mux *asynq.ServeMux, d *botworker.Deps, bc *botkit.BotConfig) {
	h := &taskHandler{d: d, botID: bc.Slug}
	mux.HandleFunc(queue.TypeSoundCloud, h.handle)
}
