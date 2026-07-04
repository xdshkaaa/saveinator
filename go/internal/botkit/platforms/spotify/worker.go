package spotifyplat

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
	"saveinator/internal/botkit/botworker"
	"saveinator/internal/cancel"
	"saveinator/internal/config"
	"saveinator/internal/locale"
	"saveinator/internal/queue"
	"saveinator/internal/spotify"
)

type taskHandler struct {
	d     *botworker.Deps
	botID string
}

func (h *taskHandler) handle(ctx context.Context, t *asynq.Task) error {
	var p queue.MusicPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		botworker.RecordTaskFailure(queue.TypeSpotify)
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
		botworker.RecordTaskFailure(queue.TypeSpotify)
		lang := p.Lang
		if lang == "" {
			lang = "en"
		}
		_ = h.d.Sender.EditMessage(p.ChatID, p.MessageID, locale.Get("spotify.download_failed", lang, nil))
		return nil
	}
	botworker.RecordTaskSuccess(queue.TypeSpotify, "spotify", time.Now(), 0)
	return nil
}

func (h *taskHandler) runMusicDownload(ctx context.Context, p queue.MusicPayload) error {
	d := h.d
	lang := p.Lang
	if lang == "" {
		lang = "en"
	}

	lockKey := fmt.Sprintf("spotify:processing:%s:%s", p.LinkType, p.ResourceID)
	ttl := releaseLockTTL(d.Cfg)
	acquired, err := d.Redis.TryAcquireReleaseLock(ctx, lockKey, ttl)
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

	return h.downloadSpotifyRelease(ctx, p, lang, cancelKB)
}

func (h *taskHandler) downloadSpotifyRelease(ctx context.Context, p queue.MusicPayload, lang string, cancelKB *telego.InlineKeyboardMarkup) error {
	d := h.d
	var release spotify.Release
	if err := json.Unmarshal([]byte(p.ReleaseJSON), &release); err != nil {
		return fmt.Errorf("decode release: %w", err)
	}
	if len(release.Tracks) == 0 {
		_ = d.Sender.EditMessage(p.ChatID, p.MessageID, locale.Get("spotify.download_none_found", lang, nil))
		return nil
	}

	taskDir, err := os.MkdirTemp("", "saveinator-spotify-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(taskDir)

	sent := 0
	total := len(release.Tracks)
	concurrency := d.Runtime.CurrentInt(ctx, "spotify.download_concurrency", d.Cfg.SpotifyDownloadConcurrency)
	trackTimeout := d.Runtime.CurrentInt(ctx, "spotify.track_timeout_sec", d.Cfg.SpotifyTrackTimeoutSeconds)
	sem := make(chan struct{}, max(1, concurrency))

	for i, track := range release.Tracks {
		if cancelled, _ := d.Redis.IsDownloadCancelled(ctx, p.LockScene, p.UserID, p.LockToken); cancelled {
			_ = d.Sender.EditMessageMarkup(p.ChatID, p.MessageID, locale.Get("download.cancelled", lang, nil), nil)
			return nil
		}

		current := i + 1
		status := locale.Get("spotify.download_track", lang, map[string]string{
			"current": fmt.Sprintf("%d", current),
			"total":   fmt.Sprintf("%d", total),
			"title":   track.Title,
		})
		_ = d.Sender.EditMessageMarkup(p.ChatID, p.MessageID, status, cancelKB)

		trackDir := filepath.Join(taskDir, fmt.Sprintf("track-%d", current))
		query := track.Artists + " - " + track.Title

		sem <- struct{}{}
		audioPath, dlErr := audio.DownloadFromYouTubeSearch(ctx, query, trackDir, d.Cfg.SpotifyDLOutputFormat, track.DurationMS, trackTimeout)
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
		_ = d.Sender.EditMessageMarkup(p.ChatID, p.MessageID, sendStatus, cancelKB)

		if err := d.Sender.SendAudio(p.ChatID, audioPath, track.Title, track.Artists, track.DurationMS/1000); err != nil {
			slog.Warn("spotify send audio failed", "title", track.Title, "err", err)
			continue
		}
		sent++
	}

	if sent == 0 {
		_ = d.Sender.EditMessageMarkup(p.ChatID, p.MessageID, locale.Get("spotify.download_none_found", lang, nil), nil)
	} else {
		_ = d.Sender.DeleteMessage(p.ChatID, p.MessageID)
	}
	url := "spotify:" + p.ResourceID
	_ = d.DB.RecordDownloadForBot(ctx, p.UserID, p.ChatID, url, "spotify", "completed", 0, "", h.botID)
	return nil
}

func (h *taskHandler) beginMusicDownloadStatus(p queue.MusicPayload, lang string, cancelKB *telego.InlineKeyboardMarkup) error {
	var release struct {
		Tracks []struct {
			Title string `json:"title"`
		} `json:"tracks"`
	}
	if err := json.Unmarshal([]byte(p.ReleaseJSON), &release); err != nil || len(release.Tracks) == 0 {
		return h.d.Sender.EditMessageMarkup(p.ChatID, p.MessageID, locale.Get("spotify.download_starting", lang, map[string]string{
			"total": "1",
		}), cancelKB)
	}
	return h.d.Sender.EditMessageMarkup(p.ChatID, p.MessageID, locale.Get("spotify.download_track", lang, map[string]string{
		"current": "1",
		"total":   fmt.Sprintf("%d", len(release.Tracks)),
		"title":   release.Tracks[0].Title,
	}), cancelKB)
}

func releaseLockTTL(cfg *config.Settings) time.Duration {
	perTrack := time.Duration(cfg.SpotifyTrackTimeoutSeconds) * time.Second
	return perTrack*time.Duration(cfg.SpotifyLockMaxTracks) + 90*time.Second
}
