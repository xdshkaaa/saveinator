package soundcloudplat

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
	"saveinator/internal/botkit/botworker"
	"saveinator/internal/cancel"
	"saveinator/internal/config"
	"saveinator/internal/locale"
	"saveinator/internal/metrics"
	"saveinator/internal/queue"
	"saveinator/internal/soundcloud"
)

type taskHandler struct {
	d     *botworker.Deps
	botID string
}

func (h *taskHandler) handle(ctx context.Context, t *asynq.Task) error {
	start := time.Now()
	var p queue.MusicPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		botworker.RecordTaskFailure(h.botID, queue.TypeSoundCloud)
		slog.Warn("soundcloud payload decode failed", "err", err)
		return nil
	}
	if p.ChatID == 0 || p.MessageID == 0 {
		slog.Warn("soundcloud payload missing chat/message", "url", p.SourceURL)
		return nil
	}
	slog.Info("soundcloud worker started", "chat", p.ChatID, "message", p.MessageID, "url", p.SourceURL)
	if err := h.runMusicDownload(ctx, p); err != nil {
		slog.Warn("soundcloud worker failed", "err", err)
		botworker.RecordTaskFailure(h.botID, queue.TypeSoundCloud)
		lang := p.Lang
		if lang == "" {
			lang = "en"
		}
		_ = h.d.Sender.EditMessage(p.ChatID, p.MessageID, locale.Get("soundcloud.download_failed", lang, nil))
		return nil
	}
	botworker.RecordTaskSuccess(h.botID, queue.TypeSoundCloud, "soundcloud", start, 0)
	return nil
}

func (h *taskHandler) runMusicDownload(ctx context.Context, p queue.MusicPayload) error {
	d := h.d
	lang := p.Lang
	if lang == "" {
		lang = "en"
	}

	lockKey := "soundcloud:processing:" + p.SourceURL
	acquired, err := d.Redis.TryAcquireReleaseLock(ctx, lockKey, releaseLockTTL(d.Cfg))
	if err != nil {
		slog.Warn("release lock error", "key", lockKey, "err", err)
	}
	if !acquired {
		slog.Info("release already downloading", "key", lockKey)
		_ = d.Sender.EditMessage(p.ChatID, p.MessageID, locale.Get("errors.busy", lang, nil))
		d.ReleaseLock(ctx, p.UserID, p.LockScene, p.LockToken)
		return nil
	}
	defer func() {
		_ = d.Redis.ReleaseReleaseLock(ctx, lockKey)
		d.ReleaseLock(ctx, p.UserID, p.LockScene, p.LockToken)
	}()

	cancelKB := cancel.Keyboard(lang, p.LockScene, p.UserID, p.LockToken)

	if err := h.beginMusicDownloadStatus(p, lang, cancelKB); err != nil {
		slog.Warn("music status update failed", "err", err)
	}

	return h.downloadSoundCloudRelease(ctx, p, lang, cancelKB)
}

func (h *taskHandler) downloadSoundCloudRelease(ctx context.Context, p queue.MusicPayload, lang string, cancelKB *telego.InlineKeyboardMarkup) error {
	d := h.d
	var release soundcloud.Release
	if err := json.Unmarshal([]byte(p.ReleaseJSON), &release); err != nil {
		return fmt.Errorf("decode release: %w", err)
	}

	taskDir, err := os.MkdirTemp("", "saveinator-soundcloud-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(taskDir)

	sent := 0
	total := len(release.Tracks)
	concurrency := d.Runtime.CurrentInt(ctx, "soundcloud.download_concurrency", d.Cfg.SoundCloudDownloadConcurrency)
	trackTimeout := d.Runtime.CurrentInt(ctx, "soundcloud.track_timeout_sec", d.Cfg.SoundCloudTrackTimeoutSeconds)
	outputFormat := d.Runtime.CurrentString(ctx, "soundcloud.audio_format", d.Cfg.SoundCloudDLOutputFormat)
	sem := make(chan struct{}, max(1, concurrency))

	for i, track := range release.Tracks {
		if cancelled, _ := d.Redis.IsDownloadCancelled(ctx, p.LockScene, p.UserID, p.LockToken); cancelled {
			_ = d.Sender.EditMessageMarkup(p.ChatID, p.MessageID, locale.Get("download.cancelled", lang, nil), nil)
			return nil
		}

		current := i + 1
		statusKey := "soundcloud.download_track"
		if track.YouTubeFallback {
			statusKey = "soundcloud.download_track_youtube"
		}
		status := locale.Get(statusKey, lang, map[string]string{
			"current": fmt.Sprintf("%d", current),
			"total":   fmt.Sprintf("%d", total),
			"title":   track.Title,
		})
		_ = d.Sender.EditMessageMarkup(p.ChatID, p.MessageID, status, cancelKB)

		trackURL := track.SoundCloudURL
		if trackURL == "" && !track.YouTubeFallback {
			continue
		}
		trackDir := filepath.Join(taskDir, fmt.Sprintf("track-%d", current))
		query := youtubeQuery(track, release)

		sem <- struct{}{}
		trackStart := time.Now()
		var audioPath string
		var dlErr error
		if track.YouTubeFallback {
			metrics.SoundCloudYouTubeFallbackTotal.Inc()
			audioPath, dlErr = audio.DownloadFromYouTubeSearch(ctx, query, trackDir, outputFormat, track.DurationMS, trackTimeout)
		} else {
			audioPath, dlErr = audio.DownloadSoundCloudTrack(ctx, trackURL, trackDir, outputFormat, trackTimeout)
			if dlErr != nil && audio.IsDRMProtectedError(dlErr) {
				metrics.SoundCloudYouTubeFallbackTotal.Inc()
				youtubeStatus := locale.Get("soundcloud.download_track_youtube", lang, map[string]string{
					"current": fmt.Sprintf("%d", current),
					"total":   fmt.Sprintf("%d", total),
					"title":   track.Title,
				})
				_ = d.Sender.EditMessageMarkup(p.ChatID, p.MessageID, youtubeStatus, cancelKB)
				audioPath, dlErr = audio.DownloadFromYouTubeSearch(ctx, query, trackDir, outputFormat, track.DurationMS, trackTimeout)
			}
		}
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
		_ = d.Sender.EditMessageMarkup(p.ChatID, p.MessageID, sendStatus, cancelKB)

		performer := firstNonEmpty(track.Artist, release.Artist)
		if err := d.Sender.SendAudio(p.ChatID, audioPath, track.Title, performer, track.DurationMS/1000, ""); err != nil {
			slog.Warn("soundcloud send audio failed", "title", track.Title, "err", err)
			metrics.SoundCloudDownloadFailuresTotal.Inc()
			continue
		}
		metrics.SoundCloudDownloadsSuccessTotal.Inc()
		sent++
	}

	if sent == 0 {
		_ = d.Sender.EditMessageMarkup(p.ChatID, p.MessageID, locale.Get("soundcloud.download_failed", lang, nil), nil)
		_ = d.DB.RecordDownloadForBot(ctx, p.UserID, p.ChatID, p.SourceURL, "soundcloud", "failed", 0, fmt.Sprintf("0/%d tracks sent", total), h.botID)
	} else {
		_ = d.Sender.DeleteMessage(p.ChatID, p.MessageID)
		_ = d.DB.RecordDownloadForBot(ctx, p.UserID, p.ChatID, p.SourceURL, "soundcloud", "completed", 0, "", h.botID)
	}
	return nil
}

func (h *taskHandler) beginMusicDownloadStatus(p queue.MusicPayload, lang string, cancelKB *telego.InlineKeyboardMarkup) error {
	var release struct {
		Tracks []struct {
			Title string `json:"title"`
		} `json:"tracks"`
	}
	if err := json.Unmarshal([]byte(p.ReleaseJSON), &release); err != nil || len(release.Tracks) == 0 {
		return h.d.Sender.EditMessageMarkup(p.ChatID, p.MessageID, locale.Get("soundcloud.download_starting", lang, map[string]string{
			"total": "1",
		}), cancelKB)
	}
	return h.d.Sender.EditMessageMarkup(p.ChatID, p.MessageID, locale.Get("soundcloud.download_track", lang, map[string]string{
		"current": "1",
		"total":   fmt.Sprintf("%d", len(release.Tracks)),
		"title":   release.Tracks[0].Title,
	}), cancelKB)
}

func releaseLockTTL(cfg *config.Settings) time.Duration {
	perTrack := time.Duration(cfg.SoundCloudTrackTimeoutSeconds) * time.Second
	return perTrack*time.Duration(cfg.SoundCloudMaxTracks) + 90*time.Second
}

func youtubeQuery(track soundcloud.Track, release soundcloud.Release) string {
	artist := firstNonEmpty(track.Artist, release.Artist)
	title := firstNonEmpty(track.Title, release.Title)
	if artist != "" && title != "" {
		return artist + " - " + title
	}
	return firstNonEmpty(title, artist)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
