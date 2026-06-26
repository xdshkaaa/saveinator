package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/hibiken/asynq"

	"saveinator/internal/locale"
	"saveinator/internal/pinterest"
	"saveinator/internal/queue"
	"saveinator/internal/tiktok"
)

func (h *Handler) handlePinterest(ctx context.Context, t *asynq.Task) error {
	var p queue.DownloadPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}
	defer h.releaseLock(ctx, p)
	return h.runPinterest(ctx, p)
}

func (h *Handler) runPinterest(ctx context.Context, p queue.DownloadPayload) error {
	lang := p.Lang
	if lang == "" {
		lang = "en"
	}

	_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("download.downloading", lang, nil))

	taskDir, err := os.MkdirTemp("", "saveinator-pin-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(taskDir)

	client := pinterest.NewClient(h.cfg.PinterestCookiesPath, h.cfg.PinterestTimeoutSeconds)
	result, err := client.Download(ctx, p.URL, taskDir, h.cfg.PinterestMaxItems, h.cfg.PinterestDownloadImages, h.cfg.PinterestDownloadVideos)
	if err != nil {
		if errors.Is(err, pinterest.ErrNoMedia) {
			_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("pinterest.no_media", lang, nil))
			return nil
		}
		slog.Warn("pinterest download failed", "err", err)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("download.timeout", lang, nil))
		return nil
	}
	if len(result.Items) == 0 {
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("pinterest.no_media", lang, nil))
		return nil
	}

	_ = h.sender.DeleteMessage(p.ChatID, p.MessageID)
	item := pickPinterestItem(result.Items)
	sizeMB := float64(item.FileSize) / (1024 * 1024)
	limit := float64(h.cfg.SendVideoLimitMB)
	if item.MediaType == "image" {
		limit = float64(h.cfg.SendDocumentLimitMB)
	}
	if sizeMB > limit {
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("pinterest.all_too_large", lang, nil))
		return nil
	}

	title := item.Title
	if title == "" {
		title = filepath.Base(item.FilePath)
	}
	animation := false
	if err := h.sender.SendFile(p.ChatID, item.FilePath, title, lang, "pinterest", animation); err != nil {
		slog.Warn("pinterest send failed", "err", err)
	}
	_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "pinterest", "completed", sizeMB, "")
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

func (h *Handler) runTikTok(ctx context.Context, p queue.DownloadPayload) error {
	lang := p.Lang
	if lang == "" {
		lang = "en"
	}

	_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("download.downloading", lang, nil))

	taskDir, err := os.MkdirTemp("", "saveinator-tiktok-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(taskDir)

	dl := tiktok.NewDownloader(
		h.cfg.TikTokCookiesPath,
		h.cfg.DownloadTimeoutSeconds,
		h.cfg.TikTokCarouselMaxItems,
		h.cfg.TikTokCarouselAudioEnabled,
	)
	result, err := dl.Download(ctx, p.URL, taskDir)
	if err != nil {
		slog.Warn("tiktok download failed", "err", err)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("errors.generic", lang, nil))
		return nil
	}

	_ = h.sender.DeleteMessage(p.ChatID, p.MessageID)
	caption := buildTikTokCaption(result.Title, result.Author, lang)

	switch result.PostType {
	case tiktok.PostTypeCarousel, tiktok.PostTypeAudioOnly:
		if len(result.Images) == 0 && result.AudioPath == "" {
			_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("tiktok.carousel_empty", lang, nil))
			return nil
		}
		_ = h.sender.SendPhotoAlbum(p.ChatID, result.Images, caption)
		if result.AudioPath != "" {
			_ = h.sender.SendAudio(p.ChatID, result.AudioPath, caption)
		}
	case tiktok.PostTypeVideo:
		if result.VideoPath == "" {
			_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("errors.generic", lang, nil))
			return nil
		}
		if err := h.sender.SendFile(p.ChatID, result.VideoPath, result.Title, lang, "tiktok", false); err != nil {
			slog.Warn("tiktok send failed", "err", err)
		}
	default:
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("errors.generic", lang, nil))
		return nil
	}

	_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "tiktok", "completed", 0, "")
	return nil
}

func buildTikTokCaption(title, author, lang string) string {
	parts := []string{}
	if title != "" {
		parts = append(parts, title)
	}
	if author != "" {
		parts = append(parts, "@"+author)
	}
	content := stringsJoin(parts, "\n")
	via := locale.Get("download.via_bot", lang, map[string]string{"bot_username": "saveinator_bot"})
	if content == "" {
		return via
	}
	return content + "\n\n" + via
}

func stringsJoin(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += sep + p
	}
	return out
}
