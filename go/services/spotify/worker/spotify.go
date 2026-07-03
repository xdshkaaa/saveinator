package worker

import (
	"context"
	"encoding/json"
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
	"saveinator/internal/queue"
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
	if err := h.runMusicDownload(ctx, p); err != nil {
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

func (h *Handler) runMusicDownload(ctx context.Context, p queue.MusicPayload) error {
	lang := p.Lang
	if lang == "" {
		lang = "en"
	}

	lockKey := fmt.Sprintf("spotify:processing:%s:%s", p.LinkType, p.ResourceID)
	ttl := releaseLockTTL(h.cfg)
	acquired, err := h.redis.TryAcquireReleaseLock(ctx, lockKey, ttl)
	if err != nil {
		slog.Warn("release lock error", "key", lockKey, "err", err)
	}
	if !acquired {
		slog.Info("release already downloading", "key", lockKey)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("errors.busy", lang, nil))
		h.releaseMusicUserLock(ctx, p)
		return nil
	}
	defer func() {
		_ = h.redis.ReleaseReleaseLock(ctx, lockKey)
		h.releaseMusicUserLock(ctx, p)
	}()

	cancelKB := cancel.Keyboard(lang, p.LockScene, p.UserID, p.LockToken)

	if err := h.beginMusicDownloadStatus(p, lang, cancelKB); err != nil {
		slog.Warn("music status update failed", "err", err)
	}

	return h.downloadSpotifyRelease(ctx, p, lang, cancelKB)
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

	if sent == 0 {
		_ = h.sender.EditMessageMarkup(p.ChatID, p.MessageID, locale.Get("spotify.download_none_found", lang, nil), nil)
	} else {
		_ = h.sender.DeleteMessage(p.ChatID, p.MessageID)
	}
	url := "spotify:" + p.ResourceID
	_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, url, "spotify", "completed", 0, "")
	return nil
}

func (h *Handler) beginMusicDownloadStatus(p queue.MusicPayload, lang string, cancelKB *telego.InlineKeyboardMarkup) error {
	var release struct {
		Tracks []struct {
			Title string `json:"title"`
		} `json:"tracks"`
	}
	if err := json.Unmarshal([]byte(p.ReleaseJSON), &release); err != nil || len(release.Tracks) == 0 {
		return h.sender.EditMessageMarkup(p.ChatID, p.MessageID, locale.Get("spotify.download_starting", lang, map[string]string{
			"total": "1",
		}), cancelKB)
	}
	return h.sender.EditMessageMarkup(p.ChatID, p.MessageID, locale.Get("spotify.download_track", lang, map[string]string{
		"current": "1",
		"total":   fmt.Sprintf("%d", len(release.Tracks)),
		"title":   release.Tracks[0].Title,
	}), cancelKB)
}

func releaseLockTTL(cfg *config.Settings) time.Duration {
	perTrack := time.Duration(cfg.SpotifyTrackTimeoutSeconds) * time.Second
	return perTrack*time.Duration(cfg.SpotifyLockMaxTracks) + 90*time.Second
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
