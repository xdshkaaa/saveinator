package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/cancel"
	"saveinator/internal/linkparser"
	"saveinator/internal/locale"
	"saveinator/internal/metrics"
	"saveinator/internal/queue"
	"saveinator/internal/soundcloud"
	"saveinator/internal/spotify"
	"saveinator/internal/tgemoji"
	"saveinator/internal/yandexmusic"
)

func (b *Bot) handleSpotifyLink(ctx context.Context, bot *telego.Bot, msg telego.Message, lang string, link linkparser.ParsedLink) {
	metrics.SpotifyRequestsTotal.Inc()
	if link.SpotifyID == "" || link.SpotifyTyp == "" {
		_, _ = bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), locale.Get("errors.unsupported", lang, nil)))
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
		_, _ = bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), locale.Get("spotify.disabled", lang, nil)))
		releaseLock()
		return
	}
	if !b.spotify.Enabled() {
		_, _ = bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), locale.Get("spotify.not_configured", lang, nil)))
		releaseLock()
		return
	}

	release, err := b.spotify.FetchRelease(link.SpotifyTyp, link.SpotifyID)
	if err != nil {
		switch err {
		case spotify.ErrNotFound:
			_, _ = bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), locale.Get("spotify.not_found", lang, nil)))
		case spotify.ErrAuth:
			_, _ = bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), locale.Get("spotify.not_configured", lang, nil)))
		default:
			_, _ = bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), locale.Get("spotify.api_error", lang, nil)))
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
	statusMsg := htmlMessage(tu.ID(msg.Chat.ID), statusText)
	if token != "" {
		statusMsg = statusMsg.WithReplyMarkup(cancel.Keyboard(lang, "spotify", msg.From.ID, token))
	}
	status, err := bot.SendMessage(statusMsg)
	if err != nil {
		releaseLock()
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
			Text:      tgemoji.Render(locale.Get("spotify.download_failed", lang, nil)),
			ParseMode: telego.ModeHTML,
		})
		releaseLock()
		return
	}
	metrics.DownloadsEnqueued.WithLabelValues("spotify").Inc()
}

func (b *Bot) handleYandexMusicLink(ctx context.Context, bot *telego.Bot, msg telego.Message, lang string, link linkparser.ParsedLink) {
	metrics.YandexMusicRequestsTotal.Inc()
	if link.YandexAlbumID == "" && link.YandexTrackID == "" {
		_, _ = bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), locale.Get("errors.unsupported", lang, nil)))
		return
	}

	token, ok, err := b.acquireUserLock(ctx, msg.From.ID, "yandexmusic", musicLockTTL(b.cfg, "yandexmusic", b.cfg.YandexMusicLockMaxTracks))
	if err != nil {
		slog.Warn("yandexmusic lock failed", "err", err)
		return
	}
	if !ok {
		b.replyBusy(ctx, bot, msg, lang, "yandexmusic")
		return
	}

	releaseLock := func() {
		if token == "" {
			return
		}
		_ = b.redis.ReleaseUserLock(ctx, msg.From.ID, "yandexmusic", token)
	}

	if !b.yandex.Enabled() {
		_, _ = bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), locale.Get("yandexmusic.not_configured", lang, nil)))
		releaseLock()
		return
	}

	release, err := b.yandex.FetchRelease(link.YandexAlbumID, link.YandexTrackID)
	if err != nil {
		b.replyYandexFetchError(bot, msg.Chat.ID, lang, err)
		releaseLock()
		return
	}

	b.sendYandexCardAndEnqueue(ctx, bot, msg.Chat.ID, msg.From.ID, lang, release, token, releaseLock)
}

// onYandexAlbumDownload handles the ym:alb:<albumID> button on single-track
// cards: it re-runs the full album flow for the referenced album.
func (b *Bot) onYandexAlbumDownload(bot *telego.Bot) func(context.Context, *telego.Bot, telego.CallbackQuery) {
	return func(ctx context.Context, _ *telego.Bot, query telego.CallbackQuery) {
		if query.From.ID == 0 || query.Message == nil || !strings.HasPrefix(query.Data, yandexmusic.AlbumCallbackPrefix) {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			return
		}
		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))

		albumID := strings.TrimPrefix(query.Data, yandexmusic.AlbumCallbackPrefix)
		if !isNumericID(albumID) {
			return
		}
		chat := query.Message.GetChat()
		lang := b.userLang(ctx, query.From.ID)

		if !b.runtime.PlatformEnabled(ctx, "yandexmusic") {
			_, _ = bot.SendMessage(htmlMessage(tu.ID(chat.ID), locale.Get("yandexmusic.disabled", lang, nil)))
			return
		}

		token, ok, err := b.acquireUserLock(ctx, query.From.ID, "yandexmusic", musicLockTTL(b.cfg, "yandexmusic", b.cfg.YandexMusicLockMaxTracks))
		if err != nil {
			slog.Warn("yandexmusic lock failed", "err", err)
			return
		}
		if !ok {
			metrics.RecordUserQueueRejected("yandexmusic")
			kb := cancel.QueueButton(lang, query.From.ID)
			_, _ = bot.SendMessage(htmlMessage(tu.ID(chat.ID), locale.Get("errors.busy", lang, nil)).WithReplyMarkup(kb))
			return
		}

		releaseLock := func() {
			if token == "" {
				return
			}
			_ = b.redis.ReleaseUserLock(ctx, query.From.ID, "yandexmusic", token)
		}

		if !b.yandex.Enabled() {
			_, _ = bot.SendMessage(htmlMessage(tu.ID(chat.ID), locale.Get("yandexmusic.not_configured", lang, nil)))
			releaseLock()
			return
		}

		release, err := b.yandex.FetchRelease(albumID, "")
		if err != nil {
			b.replyYandexFetchError(bot, chat.ID, lang, err)
			releaseLock()
			return
		}

		b.sendYandexCardAndEnqueue(ctx, bot, chat.ID, query.From.ID, lang, release, token, releaseLock)
	}
}

func (b *Bot) replyYandexFetchError(bot *telego.Bot, chatID int64, lang string, err error) {
	switch {
	case errors.Is(err, yandexmusic.ErrNotFound):
		_, _ = bot.SendMessage(htmlMessage(tu.ID(chatID), locale.Get("yandexmusic.not_found", lang, nil)))
	case errors.Is(err, yandexmusic.ErrAuth):
		_, _ = bot.SendMessage(htmlMessage(tu.ID(chatID), locale.Get("yandexmusic.not_configured", lang, nil)))
	case errors.Is(err, yandexmusic.ErrGeo):
		_, _ = bot.SendMessage(htmlMessage(tu.ID(chatID), locale.Get("yandexmusic.geoblocked", lang, nil)))
	default:
		_, _ = bot.SendMessage(htmlMessage(tu.ID(chatID), locale.Get("yandexmusic.api_error", lang, nil)))
	}
}

func (b *Bot) sendYandexCardAndEnqueue(ctx context.Context, bot *telego.Bot, chatID, userID int64, lang string, release *yandexmusic.Release, token string, releaseLock func()) {
	downloadEnabled := b.runtime.CurrentBool(ctx, "yandexmusic.download_enabled", b.cfg.YandexMusicDownloadEnabled)
	text := yandexmusic.CardText(release, lang, downloadEnabled)
	kb := yandexmusic.OpenKeyboard(release, lang)
	b.sendMusicCard(bot, chatID, release.CoverURL, text, kb)

	if !downloadEnabled || len(release.Tracks) == 0 {
		releaseLock()
		return
	}

	releaseJSON, err := json.Marshal(release)
	if err != nil {
		releaseLock()
		return
	}

	statusText := locale.Get("yandexmusic.download_starting", lang, map[string]string{
		"total": fmt.Sprintf("%d", len(release.Tracks)),
	})
	statusMsg := htmlMessage(tu.ID(chatID), statusText)
	if token != "" {
		statusMsg = statusMsg.WithReplyMarkup(cancel.Keyboard(lang, "yandexmusic", userID, token))
	}
	status, err := bot.SendMessage(statusMsg)
	if err != nil {
		releaseLock()
		return
	}

	linkType := "album"
	resourceID := release.AlbumID
	sourceURL := release.YandexURL
	if len(release.Tracks) == 1 {
		linkType = "track"
		resourceID = release.Tracks[0].SourceID
		sourceURL = release.Tracks[0].YandexURL
	}

	if err := b.q.EnqueueYandexMusic(queue.MusicPayload{
		Platform:    "yandexmusic",
		ChatID:      chatID,
		UserID:      userID,
		MessageID:   status.MessageID,
		Lang:        lang,
		LockToken:   token,
		LockScene:   "yandexmusic",
		LinkType:    linkType,
		ResourceID:  resourceID,
		SourceURL:   sourceURL,
		ReleaseJSON: string(releaseJSON),
	}); err != nil {
		slog.Warn("enqueue yandexmusic failed", "err", err)
		_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
			ChatID:    tu.ID(chatID),
			MessageID: status.MessageID,
			Text:      tgemoji.Render(locale.Get("yandexmusic.download_failed", lang, nil)),
			ParseMode: telego.ModeHTML,
		})
		releaseLock()
		return
	}
	metrics.DownloadsEnqueued.WithLabelValues("yandexmusic").Inc()
}

func isNumericID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (b *Bot) handleSoundCloudLink(ctx context.Context, bot *telego.Bot, msg telego.Message, lang string, rawURL string) {
	metrics.SoundCloudRequestsTotal.Inc()
	scLink, err := soundcloud.ParseLink(rawURL)
	if err != nil {
		_, _ = bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), locale.Get("errors.unsupported", lang, nil)))
		return
	}

	token, ok, err := b.acquireUserLock(ctx, msg.From.ID, "soundcloud", musicLockTTL(b.cfg, "soundcloud", b.cfg.SoundCloudMaxTracks))
	if err != nil {
		slog.Warn("soundcloud lock failed", "err", err)
		return
	}
	if !ok {
		b.replyBusy(ctx, bot, msg, lang, "soundcloud")
		return
	}

	releaseLock := func() {
		if token == "" {
			return
		}
		_ = b.redis.ReleaseUserLock(ctx, msg.From.ID, "soundcloud", token)
	}

	if !b.cfg.SoundCloudEnabled {
		_, _ = bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), locale.Get("soundcloud.disabled", lang, nil)))
		releaseLock()
		return
	}

	maxTracks := b.runtime.CurrentInt(ctx, "soundcloud.max_tracks_per_playlist", b.cfg.SoundCloudMaxTracks)
	release, err := b.soundcloud.FetchRelease(ctx, scLink, maxTracks)
	if err != nil {
		switch {
		case errors.Is(err, soundcloud.ErrNotFound):
			_, _ = bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), locale.Get("soundcloud.not_found", lang, nil)))
		case errors.Is(err, soundcloud.ErrTooLarge):
			_, _ = bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), locale.Get("soundcloud.playlist_too_large", lang, map[string]string{
				"limit": fmt.Sprintf("%d", maxTracks),
			})))
		default:
			metrics.SoundCloudMetadataFailuresTotal.Inc()
			_, _ = bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), locale.Get("soundcloud.download_failed", lang, nil)))
		}
		releaseLock()
		return
	}

	downloadEnabled := b.runtime.CurrentBool(ctx, "soundcloud.download_enabled", b.cfg.SoundCloudDownloadEnabled)
	text := soundcloud.CardText(release, lang, downloadEnabled)
	kb := soundcloud.OpenKeyboard(release, lang)
	b.sendMusicCard(bot, msg.Chat.ID, release.ArtworkURL, text, kb)

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
	statusMsg := htmlMessage(tu.ID(msg.Chat.ID), statusText)
	if token != "" {
		statusMsg = statusMsg.WithReplyMarkup(cancel.Keyboard(lang, "soundcloud", msg.From.ID, token))
	}
	status, err := bot.SendMessage(statusMsg)
	if err != nil {
		releaseLock()
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
			Text:      tgemoji.Render(locale.Get("soundcloud.download_failed", lang, nil)),
			ParseMode: telego.ModeHTML,
		})
		releaseLock()
		return
	}
	metrics.DownloadsEnqueued.WithLabelValues("soundcloud").Inc()
	metrics.SoundCloudDownloadsEnqueuedTotal.Inc()
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
	_, _ = bot.SendMessage(htmlMessage(tu.ID(chatID), text).WithReplyMarkup(kb))
}

func (b *Bot) replyBusy(_ context.Context, bot messageSender, msg telego.Message, lang, scenario string) {
	metrics.RecordUserQueueRejected(scenario)
	kb := cancel.QueueButton(lang, msg.From.ID)
	_, _ = bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), locale.Get("errors.busy", lang, nil)).WithReplyMarkup(kb))
}
