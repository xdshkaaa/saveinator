package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/cancel"
	"saveinator/internal/linkparser"
	"saveinator/internal/locale"
	"saveinator/internal/metrics"
	"saveinator/internal/queue"
	"saveinator/internal/soundcloud"
	"saveinator/internal/spotify"
)

func (b *Bot) handleSpotifyLink(ctx context.Context, bot *telego.Bot, msg telego.Message, lang string, link linkparser.ParsedLink) {
	if link.SpotifyID == "" || link.SpotifyTyp == "" {
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("errors.unsupported", lang, nil)))
		return
	}

	token, ok, err := b.redis.AcquireUserLock(ctx, msg.From.ID, "spotify", musicLockTTL(b.cfg, "spotify", b.cfg.SpotifyLockMaxTracks))
	if err != nil {
		slog.Warn("spotify lock failed", "err", err)
		return
	}
	if !ok {
		b.replyBusy(ctx, bot, msg, lang)
		return
	}

	releaseLock := func() {
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

	downloadEnabled := b.cfg.SpotifyDownloadEnabled
	text := spotify.CardText(release, lang, downloadEnabled)
	kb := spotify.OpenKeyboard(release, lang)
	b.sendMusicCard(bot, msg.Chat.ID, release.CoverURL, text, kb)

	releaseLock()

	if !downloadEnabled || len(release.Tracks) == 0 {
		return
	}

	releaseJSON, err := json.Marshal(release)
	if err != nil {
		return
	}

	statusText := locale.Get("spotify.download_starting", lang, map[string]string{
		"total": fmt.Sprintf("%d", len(release.Tracks)),
	})
	status, err := bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), statusText).WithReplyMarkup(
		cancel.Keyboard(lang, "spotify", msg.From.ID, token),
	))
	if err != nil {
		return
	}

	if err := b.q.EnqueueSpotify(queue.MusicPayload{
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
		return
	}
	metrics.DownloadsEnqueued.WithLabelValues("spotify").Inc()
}

func (b *Bot) handleSoundCloudLink(ctx context.Context, bot *telego.Bot, msg telego.Message, lang string, rawURL string) {
	scLink, err := soundcloud.ParseLink(rawURL)
	if err != nil {
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("errors.unsupported", lang, nil)))
		return
	}

	token, ok, err := b.redis.AcquireUserLock(ctx, msg.From.ID, "soundcloud", musicLockTTL(b.cfg, "soundcloud", b.cfg.SoundCloudMaxTracks))
	if err != nil {
		slog.Warn("soundcloud lock failed", "err", err)
		return
	}
	if !ok {
		b.replyBusy(ctx, bot, msg, lang)
		return
	}

	releaseLock := func() {
		_ = b.redis.ReleaseUserLock(ctx, msg.From.ID, "soundcloud", token)
	}

	if !b.cfg.SoundCloudEnabled {
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("soundcloud.disabled", lang, nil)))
		releaseLock()
		return
	}

	release, err := b.soundcloud.FetchRelease(ctx, scLink)
	if err != nil {
		switch {
		case err == soundcloud.ErrNotFound:
			_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("soundcloud.not_found", lang, nil)))
		case err == soundcloud.ErrTooLarge:
			_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("soundcloud.playlist_too_large", lang, map[string]string{
				"limit": fmt.Sprintf("%d", b.cfg.SoundCloudMaxTracks),
			})))
		default:
			_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("soundcloud.download_failed", lang, nil)))
		}
		releaseLock()
		return
	}

	downloadEnabled := b.cfg.SoundCloudDownloadEnabled
	text := soundcloud.CardText(release, lang, downloadEnabled)
	kb := soundcloud.OpenKeyboard(release, lang)
	b.sendMusicCard(bot, msg.Chat.ID, release.ArtworkURL, text, kb)

	releaseLock()

	if !downloadEnabled || len(release.Tracks) == 0 {
		return
	}

	releaseJSON, err := json.Marshal(release)
	if err != nil {
		return
	}

	statusText := locale.Get("soundcloud.download_starting", lang, map[string]string{
		"total": fmt.Sprintf("%d", len(release.Tracks)),
	})
	status, err := bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), statusText).WithReplyMarkup(
		cancel.Keyboard(lang, "soundcloud", msg.From.ID, token),
	))
	if err != nil {
		return
	}

	if err := b.q.EnqueueSoundCloud(queue.MusicPayload{
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
		_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
			ChatID:    tu.ID(msg.Chat.ID),
			MessageID: status.MessageID,
			Text:      locale.Get("soundcloud.download_failed", lang, nil),
		})
		return
	}
	metrics.DownloadsEnqueued.WithLabelValues("soundcloud").Inc()
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

func (b *Bot) replyBusy(_ context.Context, bot *telego.Bot, msg telego.Message, lang string) {
	kb := cancel.QueueButton(lang, msg.From.ID)
	_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("errors.busy", lang, nil)).WithReplyMarkup(kb))
}
