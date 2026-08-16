package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hibiken/asynq"

	"saveinator/internal/locale"
	"saveinator/internal/metrics"
	"saveinator/internal/queue"
	"saveinator/internal/ytdlp"
)

func (h *Handler) handleInstagram(ctx context.Context, t *asynq.Task) error {
	var p queue.DownloadPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}
	p.Platform = "instagram"
	defer h.releaseLock(ctx, p)
	if h.checkCancelled(ctx, p) {
		return nil
	}
	return h.runInstagram(ctx, p)
}

func (h *Handler) runInstagram(ctx context.Context, p queue.DownloadPayload) error {
	start := time.Now()
	lang := p.Lang
	if lang == "" {
		lang = "en"
	}

	_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("download.downloading", lang, nil))

	taskDir, err := os.MkdirTemp("", "saveinator-instagram-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(taskDir)

	timeout := time.Duration(h.runtime.PlatformTimeoutSec(ctx, "instagram")) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	dlCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := ytdlp.Download(dlCtx, p.URL, taskDir, h.ytdlpOpts("instagram", "best", timeout)); err != nil {
		slog.Warn("instagram download failed", "url", p.URL, "err", err)
		metrics.RecordYtdlpError("instagram")
		recordTaskFailure(queue.TypeInstagram)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, err))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "instagram", "failed", 0, err.Error())
		return nil
	}

	files, err := ytdlp.FindMediaFiles(taskDir)
	if err != nil {
		recordTaskFailure(queue.TypeInstagram)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, err))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "instagram", "failed", 0, err.Error())
		return nil
	}

	sourceVideo := ytdlp.LargestVideo(files)
	images := ytdlp.ImageFiles(files)
	if sourceVideo == "" && len(images) == 0 {
		recordTaskFailure(queue.TypeInstagram)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("instagram.no_media", lang, nil))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "instagram", "failed", 0, "no media found")
		return nil
	}

	if sourceVideo == "" {
		// Photo post or carousel: send as an album (chunked by 10 in
		// sender.SendPhotoAlbum). yt-dlp names slides in order, so the
		// filename sort preserves the original sequence.
		h.sendInstagramPhotos(ctx, p, lang, images, start)
	} else {
		h.sendInstagramVideo(ctx, p, lang, sourceVideo, start)
	}

	_ = h.sender.DeleteMessage(p.ChatID, p.MessageID)
	return nil
}

func (h *Handler) sendInstagramVideo(ctx context.Context, p queue.DownloadPayload, lang, videoPath string, start time.Time) {
	title := instagramTitle(videoPath)
	if err := h.sender.SendFile(p.ChatID, videoPath, title, lang, "instagram", false); err != nil {
		slog.Warn("instagram send failed", "err", err)
		recordTaskFailure(queue.TypeInstagram)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, err))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "instagram", "failed", 0, err.Error())
		return
	}
	_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "instagram", "completed", 0, "")
	recordTaskSuccess(queue.TypeInstagram, "instagram", start, fileSize(videoPath))
}

func (h *Handler) sendInstagramPhotos(ctx context.Context, p queue.DownloadPayload, lang string, images []string, start time.Time) {
	maxItems := h.runtime.CurrentInt(ctx, "instagram.carousel_max_items", 20)
	if len(images) > maxItems {
		images = images[:maxItems]
	}
	caption := buildMediaCaption("", lang)
	if err := h.sender.SendPhotoAlbum(p.ChatID, images, caption); err != nil {
		slog.Warn("instagram album send failed", "err", err)
		recordTaskFailure(queue.TypeInstagram)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, err))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "instagram", "failed", 0, err.Error())
		return
	}
	_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "instagram", "completed", 0, "")
	recordTaskSuccess(queue.TypeInstagram, "instagram", start, 0)
}

// instagramTitle returns the file's base name without extension; yt-dlp
// names files "<title>_<id>.<ext>" and the title itself is the post caption.
func instagramTitle(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}
