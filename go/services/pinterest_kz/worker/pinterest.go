package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/hibiken/asynq"

	"saveinator/internal/locale"
	"saveinator/internal/pinterest"
	"saveinator/internal/queue"
)

func (h *Handler) handlePinterest(ctx context.Context, t *asynq.Task) error {
	var p queue.DownloadPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}
	defer h.releaseLock(ctx, p)
	if h.checkCancelled(ctx, p) {
		return nil
	}
	return h.runPinterest(ctx, p)
}

func (h *Handler) checkCancelled(ctx context.Context, p queue.DownloadPayload) bool {
	if p.LockToken == "" || p.LockScene == "" {
		return false
	}
	cancelled, err := h.redis.IsDownloadCancelled(ctx, p.LockScene, p.UserID, p.LockToken)
	if err != nil || !cancelled {
		return false
	}
	lang := p.Lang
	if lang == "" {
		lang = "kk"
	}
	_ = h.sender.EditMessageMarkup(p.ChatID, p.MessageID, locale.Get("download.cancelled", lang, nil), nil)
	return true
}

func (h *Handler) runPinterest(ctx context.Context, p queue.DownloadPayload) error {
	start := time.Now()
	lang := p.Lang
	if lang == "" {
		lang = "kk"
	}

	_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("download.downloading", lang, nil))

	taskDir, err := os.MkdirTemp("", "saveinator-pin-kz-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(taskDir)

	client := pinterest.NewClient(h.cfg.PinterestCookiesPath, h.runtime.CurrentInt(ctx, "pinterest.timeout_sec", h.cfg.PinterestTimeoutSeconds))
	maxItems := h.runtime.CurrentInt(ctx, "pinterest.max_items_per_board", h.cfg.PinterestMaxItems)
	downloadImages := h.runtime.CurrentBool(ctx, "pinterest.download_images", h.cfg.PinterestDownloadImages)
	downloadVideos := h.runtime.CurrentBool(ctx, "pinterest.download_videos", h.cfg.PinterestDownloadVideos)
	result, err := client.Download(ctx, p.URL, taskDir, maxItems, downloadImages, downloadVideos)
	if err != nil {
		if errors.Is(err, pinterest.ErrNoMedia) {
			_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("pinterest.no_media", lang, nil))
			recordTaskFailure(queue.TypePinterestKZ)
			_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "pinterest", "failed", 0, "no media")
			return nil
		}
		slog.Warn("pinterest download failed", "err", err)
		recordTaskFailure(queue.TypePinterestKZ)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, err))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "pinterest", "failed", 0, err.Error())
		return nil
	}
	if len(result.Items) == 0 {
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("pinterest.no_media", lang, nil))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "pinterest", "failed", 0, "no media")
		return nil
	}

	item := pickPinterestItem(result.Items)
	sizeMB := float64(item.FileSize) / (1024 * 1024)
	limit := float64(h.runtime.PlatformMaxFileMB(ctx, "pinterest"))
	if item.MediaType == "image" {
		limit = float64(h.runtime.CurrentInt(ctx, "global.document_limit_mb", h.cfg.SendDocumentLimitMB))
	}
	if sizeMB > limit {
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("pinterest.all_too_large", lang, nil))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "pinterest", "failed", sizeMB, "too large")
		return nil
	}

	title := item.Title
	if title == "" {
		title = pinterest.DisplayTitle(item.FilePath)
	}
	if _, err := os.Stat(item.FilePath); err != nil {
		slog.Warn("pinterest media file missing", "path", item.FilePath, "err", err)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, err))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "pinterest", "failed", 0, err.Error())
		return nil
	}
	if err := h.sender.SendFile(p.ChatID, item.FilePath, title, lang, "pinterest", false); err != nil {
		slog.Warn("pinterest send failed", "err", err)
		recordTaskFailure(queue.TypePinterestKZ)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, err))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "pinterest", "failed", sizeMB, err.Error())
		return nil
	}
	_ = h.sender.DeleteMessage(p.ChatID, p.MessageID)
	_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "pinterest", "completed", sizeMB, "")
	recordTaskSuccess(queue.TypePinterestKZ, "pinterest", start, item.FileSize)
	return nil
}

func pickPinterestItem(items []pinterest.MediaItem) pinterest.MediaItem {
	var best pinterest.MediaItem
	for _, item := range items {
		if item.MediaType == "video" {
			return item
		}
		if item.FileSize > best.FileSize {
			best = item
		}
	}
	if best.FilePath != "" {
		return best
	}
	return items[0]
}
