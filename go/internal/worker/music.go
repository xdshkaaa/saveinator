package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/hibiken/asynq"
	"github.com/mymmrac/telego"

	"saveinator/internal/audio"
	"saveinator/internal/cancel"
	"saveinator/internal/config"
	"saveinator/internal/locale"
	"saveinator/internal/metrics"
	"saveinator/internal/queue"
	"saveinator/internal/soundcloud"
	"saveinator/internal/spotify"
)

func (h *Handler) handleSpotify(ctx context.Context, t *asynq.Task) error {
	var p queue.MusicPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		recordTaskFailure(queue.TypeSpotify)
		slog.Warn("spotify payload decode failed", "err", err)
		return nil
	}
	if p.ChatID == 0 || p.MessageID == 0 {
		slog.Warn("spotify payload missing chat/message", "resource", p.ResourceID)
		return nil
	}
	slog.Info("spotify worker started", "chat", p.ChatID, "message", p.MessageID, "resource", p.ResourceID)
	if err := h.runMusicDownload(ctx, p, "spotify"); err != nil {
		slog.Warn("spotify worker failed", "err", err)
		recordTaskFailure(queue.TypeSpotify)
		lang := p.Lang
		if lang == "" {
			lang = "en"
		}
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("spotify.download_failed", lang, nil))
		return nil
	}
	recordTaskSuccess(queue.TypeSpotify, "spotify", time.Now(), 0)
	return nil
}

func (h *Handler) handleSoundCloud(ctx context.Context, t *asynq.Task) error {
	start := time.Now()
	var p queue.MusicPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		recordTaskFailure(queue.TypeSoundCloud)
		slog.Warn("soundcloud payload decode failed", "err", err)
		return nil
	}
	if err := h.runMusicDownload(ctx, p, "soundcloud"); err != nil {
		slog.Warn("soundcloud worker failed", "err", err)
		recordTaskFailure(queue.TypeSoundCloud)
		lang := p.Lang
		if lang == "" {
			lang = "en"
		}
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("soundcloud.download_failed", lang, nil))
		return nil
	}
	recordTaskSuccess(queue.TypeSoundCloud, "soundcloud", start, 0)
	return nil
}

func (h *Handler) runMusicDownload(ctx context.Context, p queue.MusicPayload, platform string) error {
	lang := p.Lang
	if lang == "" {
		lang = "en"
	}

	lockKey := h.releaseLockKey(platform, p)
	acquired, err := h.redis.TryAcquireReleaseLock(ctx, lockKey, releaseLockTTL(platform, h.cfg))
	if err != nil {
		slog.Warn("release lock error", "platform", platform, "key", lockKey, "err", err)
	}
	if !acquired {
		slog.Info("release already downloading", "platform", platform, "key", lockKey)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("errors.busy", lang, nil))
		h.releaseMusicUserLock(ctx, p)
		return nil
	}
	defer func() {
		_ = h.redis.ReleaseReleaseLock(ctx, lockKey)
		h.releaseMusicUserLock(ctx, p)
	}()

	cancelKB := cancel.Keyboard(lang, p.LockScene, p.UserID, p.LockToken)

	if err := h.beginMusicDownloadStatus(p, lang, platform, cancelKB); err != nil {
		slog.Warn("music status update failed", "platform", platform, "err", err)
	}

	switch platform {
	case "spotify":
		return h.downloadSpotifyRelease(ctx, p, lang, cancelKB)
	case "soundcloud":
		return h.downloadSoundCloudRelease(ctx, p, lang, cancelKB)
	default:
		return nil
	}
}

func (h *Handler) downloadSpotifyRelease(ctx context.Context, p queue.MusicPayload, lang string, cancelKB *telego.InlineKeyboardMarkup) error {
	var release spotify.Release
	if err := json.Unmarshal([]byte(p.ReleaseJSON), &release); err != nil {
		return fmt.Errorf("decode release: %w", err)
	}
	if len(release.Tracks) == 0 {
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("spotify.download_none_found", lang, nil))
		return nil
	}

	taskDir, err := os.MkdirTemp("", "saveinator-spotify-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(taskDir)

	sent := 0
	total := len(release.Tracks)
	concurrency := h.runtime.CurrentInt(ctx, "spotify.download_concurrency", h.cfg.SpotifyDownloadConcurrency)
	trackTimeout := h.runtime.CurrentInt(ctx, "spotify.track_timeout_sec", h.cfg.SpotifyTrackTimeoutSeconds)
	sem := make(chan struct{}, maxInt(1, concurrency))

	for i, track := range release.Tracks {
		if cancelled, _ := h.redis.IsDownloadCancelled(ctx, p.LockScene, p.UserID, p.LockToken); cancelled {
			_ = h.sender.EditMessageMarkup(p.ChatID, p.MessageID, locale.Get("download.cancelled", lang, nil), nil)
			return nil
		}

		current := i + 1
		status := locale.Get("spotify.download_track", lang, map[string]string{
			"current": fmt.Sprintf("%d", current),
			"total":   fmt.Sprintf("%d", total),
			"title":   track.Title,
		})
		_ = h.sender.EditMessageMarkup(p.ChatID, p.MessageID, status, cancelKB)

		trackDir := filepath.Join(taskDir, fmt.Sprintf("track-%d", current))
		query := track.Artists + " - " + track.Title

		sem <- struct{}{}
		audioPath, dlErr := audio.DownloadFromYouTubeSearch(ctx, query, trackDir, h.cfg.SpotifyDLOutputFormat, trackTimeout)
		<-sem
		if dlErr != nil {
			slog.Warn("spotify track download failed", "title", track.Title, "err", dlErr)
			continue
		}

		sendStatus := locale.Get("spotify.send_track", lang, map[string]string{
			"current": fmt.Sprintf("%d", current),
			"total":   fmt.Sprintf("%d", total),
			"title":   track.Title,
		})
		_ = h.sender.EditMessageMarkup(p.ChatID, p.MessageID, sendStatus, cancelKB)

		if err := h.sender.SendAudio(p.ChatID, audioPath, track.Title, track.Artists, track.DurationMS/1000); err != nil {
			slog.Warn("spotify send audio failed", "title", track.Title, "err", err)
			continue
		}
		sent++
	}

	return h.finishMusicDownload(ctx, p, lang, sent, total, "spotify")
}

func (h *Handler) downloadSoundCloudRelease(ctx context.Context, p queue.MusicPayload, lang string, cancelKB *telego.InlineKeyboardMarkup) error {
	var release soundcloud.Release
	if err := json.Unmarshal([]byte(p.ReleaseJSON), &release); err != nil {
		return err
	}

	taskDir, err := os.MkdirTemp("", "saveinator-soundcloud-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(taskDir)

	sent := 0
	total := len(release.Tracks)
	concurrency := h.runtime.CurrentInt(ctx, "soundcloud.download_concurrency", h.cfg.SoundCloudDownloadConcurrency)
	trackTimeout := h.runtime.CurrentInt(ctx, "soundcloud.track_timeout_sec", h.cfg.SoundCloudTrackTimeoutSeconds)
	outputFormat := h.runtime.CurrentString(ctx, "soundcloud.audio_format", h.cfg.SoundCloudDLOutputFormat)
	sem := make(chan struct{}, maxInt(1, concurrency))

	for i, track := range release.Tracks {
		if cancelled, _ := h.redis.IsDownloadCancelled(ctx, p.LockScene, p.UserID, p.LockToken); cancelled {
			_ = h.sender.EditMessageMarkup(p.ChatID, p.MessageID, locale.Get("download.cancelled", lang, nil), nil)
			return nil
		}

		current := i + 1
		status := locale.Get("soundcloud.download_track", lang, map[string]string{
			"current": fmt.Sprintf("%d", current),
			"total":   fmt.Sprintf("%d", total),
			"title":   track.Title,
		})
		_ = h.sender.EditMessageMarkup(p.ChatID, p.MessageID, status, cancelKB)

		trackURL := track.SoundCloudURL
		if trackURL == "" {
			continue
		}
		trackDir := filepath.Join(taskDir, fmt.Sprintf("track-%d", current))

		sem <- struct{}{}
		trackStart := time.Now()
		audioPath, dlErr := audio.DownloadSoundCloudTrack(ctx, trackURL, trackDir, outputFormat, trackTimeout)
		<-sem
		if dlErr != nil {
			slog.Warn("soundcloud track download failed", "title", track.Title, "err", dlErr)
			metrics.SoundCloudDownloadFailuresTotal.Inc()
			if errors.Is(dlErr, context.DeadlineExceeded) {
				metrics.SoundCloudDownloadsTimeoutTotal.Inc()
			}
			continue
		}
		metrics.SoundCloudDownloadDurationSeconds.Observe(time.Since(trackStart).Seconds())

		sendStatus := locale.Get("soundcloud.send_track", lang, map[string]string{
			"current": fmt.Sprintf("%d", current),
			"total":   fmt.Sprintf("%d", total),
			"title":   track.Title,
		})
		_ = h.sender.EditMessageMarkup(p.ChatID, p.MessageID, sendStatus, cancelKB)

		performer := firstNonEmpty(track.Artist, release.Artist)
		if err := h.sender.SendAudio(p.ChatID, audioPath, track.Title, performer, track.DurationMS/1000); err != nil {
			slog.Warn("soundcloud send audio failed", "title", track.Title, "err", err)
			metrics.SoundCloudDownloadFailuresTotal.Inc()
			continue
		}
		metrics.SoundCloudDownloadsSuccessTotal.Inc()
		sent++
	}

	return h.finishMusicDownload(ctx, p, lang, sent, total, "soundcloud")
}

func (h *Handler) finishMusicDownload(ctx context.Context, p queue.MusicPayload, lang string, sent, total int, platform string) error {
	var final string
	if sent == 0 {
		if platform == "spotify" {
			final = locale.Get("spotify.download_none_found", lang, nil)
		} else {
			final = locale.Get("soundcloud.download_failed", lang, nil)
		}
	} else {
		final = locale.Get(platform+".download_done", lang, map[string]string{
			"count": fmt.Sprintf("%d", sent),
			"total": fmt.Sprintf("%d", total),
		})
	}
	_ = h.sender.EditMessageMarkup(p.ChatID, p.MessageID, final, nil)
	url := p.SourceURL
	if url == "" {
		url = platform + ":" + p.ResourceID
	}
	_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, url, platform, "completed", 0, "")
	return nil
}

func (h *Handler) beginMusicDownloadStatus(p queue.MusicPayload, lang, platform string, cancelKB *telego.InlineKeyboardMarkup) error {
	var release struct {
		Tracks []struct {
			Title string `json:"title"`
		} `json:"tracks"`
	}
	if err := json.Unmarshal([]byte(p.ReleaseJSON), &release); err != nil || len(release.Tracks) == 0 {
		return h.sender.EditMessageMarkup(p.ChatID, p.MessageID, locale.Get(platform+".download_starting", lang, map[string]string{
			"total": "1",
		}), cancelKB)
	}
	return h.sender.EditMessageMarkup(p.ChatID, p.MessageID, locale.Get(platform+".download_track", lang, map[string]string{
		"current": "1",
		"total":   fmt.Sprintf("%d", len(release.Tracks)),
		"title":   release.Tracks[0].Title,
	}), cancelKB)
}

func (h *Handler) releaseMusicUserLock(ctx context.Context, p queue.MusicPayload) {
	if p.LockToken == "" || p.LockScene == "" {
		return
	}
	_ = h.redis.ReleaseUserLock(ctx, p.UserID, p.LockScene, p.LockToken)
}

func (h *Handler) releaseLockKey(platform string, p queue.MusicPayload) string {
	switch platform {
	case "spotify":
		return fmt.Sprintf("spotify:processing:%s:%s", p.LinkType, p.ResourceID)
	case "soundcloud":
		return "soundcloud:processing:" + p.SourceURL
	default:
		return platform + ":processing"
	}
}

func releaseLockTTL(platform string, cfg *config.Settings) time.Duration {
	switch platform {
	case "spotify":
		perTrack := time.Duration(cfg.SpotifyTrackTimeoutSeconds) * time.Second
		return perTrack*time.Duration(cfg.SpotifyLockMaxTracks) + 90*time.Second
	case "soundcloud":
		perTrack := time.Duration(cfg.SoundCloudTrackTimeoutSeconds) * time.Second
		return perTrack*time.Duration(cfg.SoundCloudMaxTracks) + 90*time.Second
	default:
		return 30 * time.Minute
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
