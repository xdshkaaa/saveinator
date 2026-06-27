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

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/locale"
	"saveinator/internal/metrics"
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
	if h.checkCancelled(ctx, p) {
		return nil
	}
	return h.runPinterest(ctx, p)
}

func (h *Handler) runPinterest(ctx context.Context, p queue.DownloadPayload) error {
	start := time.Now()
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

	client := pinterest.NewClient(h.cfg.PinterestCookiesPath, h.runtime.CurrentInt(ctx, "pinterest.timeout_sec", h.cfg.PinterestTimeoutSeconds))
	maxItems := h.runtime.CurrentInt(ctx, "pinterest.max_items_per_board", h.cfg.PinterestMaxItems)
	downloadImages := h.runtime.CurrentBool(ctx, "pinterest.download_images", h.cfg.PinterestDownloadImages)
	downloadVideos := h.runtime.CurrentBool(ctx, "pinterest.download_videos", h.cfg.PinterestDownloadVideos)
	result, err := client.Download(ctx, p.URL, taskDir, maxItems, downloadImages, downloadVideos)
	if err != nil {
		if errors.Is(err, pinterest.ErrNoMedia) {
			_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("pinterest.no_media", lang, nil))
			recordTaskFailure(queue.TypePinterest)
			return nil
		}
		slog.Warn("pinterest download failed", "err", err)
		recordTaskFailure(queue.TypePinterest)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("download.timeout", lang, nil))
		return nil
	}
	if len(result.Items) == 0 {
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("pinterest.no_media", lang, nil))
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
		return nil
	}

	title := item.Title
	if title == "" {
		title = filepath.Base(item.FilePath)
	}
	if _, err := os.Stat(item.FilePath); err != nil {
		slog.Warn("pinterest media file missing", "path", item.FilePath, "err", err)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("errors.generic", lang, nil))
		return nil
	}
	if err := h.sender.SendFile(p.ChatID, item.FilePath, title, lang, "pinterest", false); err != nil {
		slog.Warn("pinterest send failed", "err", err)
		recordTaskFailure(queue.TypePinterest)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("errors.generic", lang, nil))
		return nil
	}
	_ = h.sender.DeleteMessage(p.ChatID, p.MessageID)
	_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "pinterest", "completed", sizeMB, "")
	recordTaskSuccess(queue.TypePinterest, "pinterest", start, item.FileSize)
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
	start := time.Now()
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
		h.cfg.TikTokCookiesFromBrowser,
		h.runtime.PlatformTimeoutSec(ctx, "tiktok"),
		h.runtime.CurrentInt(ctx, "tiktok.carousel_max_items", h.cfg.TikTokCarouselMaxItems),
		h.runtime.CurrentBool(ctx, "tiktok.carousel_audio_enabled", h.cfg.TikTokCarouselAudioEnabled),
	)
	result, err := dl.Download(ctx, p.URL, taskDir)
	if err != nil {
		slog.Warn("tiktok download failed", "err", err)
		metrics.TikTokCarouselFailuresTotal.WithLabelValues("download").Inc()
		recordTaskFailure(queue.TypeTikTok)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("errors.generic", lang, nil))
		return nil
	}

	caption := buildTikTokCaption(result.Title, result.Author, lang)

	switch result.PostType {
	case tiktok.PostTypeCarousel, tiktok.PostTypeAudioOnly:
		if len(result.Images) == 0 && result.AudioPath == "" {
			_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("tiktok.carousel_empty", lang, nil))
			return nil
		}
		if err := h.sender.SendPhotoAlbum(p.ChatID, result.Images, caption); err != nil {
			slog.Warn("tiktok carousel send failed", "err", err)
			_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("errors.generic", lang, nil))
			return nil
		}
		if result.AudioPath != "" {
			if err := h.sender.SendAudio(p.ChatID, result.AudioPath, result.Title, result.Author, 0); err != nil {
				slog.Warn("tiktok audio send failed", "err", err)
			}
		}
	case tiktok.PostTypeVideo:
		if result.VideoPath == "" {
			_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("errors.generic", lang, nil))
			return nil
		}
		var markup *telego.InlineKeyboardMarkup
		if result.CarouselImagesAvailable {
			markup = h.saveCarouselSession(ctx, p, result)
		}
		var sendErr error
		if markup != nil {
			sendErr = h.sender.SendFileWithMarkup(p.ChatID, result.VideoPath, result.Title, lang, "tiktok", false, markup)
		} else {
			sendErr = h.sender.SendFile(p.ChatID, result.VideoPath, result.Title, lang, "tiktok", false)
		}
		if sendErr != nil {
			slog.Warn("tiktok send failed", "err", sendErr)
			_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("errors.generic", lang, nil))
			return nil
		}
	default:
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("errors.generic", lang, nil))
		return nil
	}

	_ = h.sender.DeleteMessage(p.ChatID, p.MessageID)
	_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "tiktok", "completed", 0, "")
	recordTaskSuccess(queue.TypeTikTok, "tiktok", start, 0)
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

func (h *Handler) saveCarouselSession(ctx context.Context, p queue.DownloadPayload, result *tiktok.Result) *telego.InlineKeyboardMarkup {
	token := uuid.NewString()[:12]
	session := &tiktok.CarouselSession{
		UserID: p.UserID,
		URL:    p.URL,
		ChatID: p.ChatID,
		Lang:   p.Lang,
		Title:  result.Title,
		Author: result.Author,
		Token:  token,
	}
	if err := h.ttSessions.Save(ctx, session); err != nil {
		slog.Warn("save carousel session failed", "err", err)
		return nil
	}
	lang := p.Lang
	if lang == "" {
		lang = "en"
	}
	return tiktok.CarouselImagesKeyboard(lang, p.UserID, token)
}

func (h *Handler) runTikTokCarouselImages(ctx context.Context, p queue.DownloadPayload) error {
	start := time.Now()
	lang := p.Lang
	if lang == "" {
		lang = "en"
	}
	defer func() {
		if p.SessionToken != "" {
			_ = h.ttSessions.Delete(ctx, p.SessionToken)
		}
	}()

	taskDir, err := os.MkdirTemp("", "saveinator-tiktok-carousel-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(taskDir)

	maxItems := h.runtime.CurrentInt(ctx, "tiktok.carousel_max_items", h.cfg.TikTokCarouselMaxItems)
	dl := tiktok.NewDownloader(
		h.cfg.TikTokCookiesPath,
		h.cfg.TikTokCookiesFromBrowser,
		h.runtime.PlatformTimeoutSec(ctx, "tiktok"),
		maxItems,
		false,
	)
	result, err := dl.DownloadCarouselImages(ctx, p.URL, taskDir)
	if err != nil || len(result.Images) == 0 {
		metrics.TikTokCarouselFailuresTotal.WithLabelValues("empty").Inc()
		recordTaskFailure(queue.TypeTikTokCarousel)
		_, _ = h.bot.SendMessage(tu.Message(tu.ID(p.ChatID), locale.Get("tiktok.carousel_empty", lang, nil)))
		return nil
	}

	if result.CarouselImageCount > 0 && len(result.Images) < result.CarouselImageCount {
		_, _ = h.bot.SendMessage(tu.Message(tu.ID(p.ChatID), locale.Get("tiktok.carousel_partial", lang, map[string]string{
			"count": fmt.Sprintf("%d", len(result.Images)),
			"total": fmt.Sprintf("%d", result.CarouselImageCount),
		})))
	}

	caption := buildTikTokCaption(result.Title, result.Author, lang)
	_ = h.sender.SendPhotoAlbum(p.ChatID, result.Images, caption)
	metrics.TikTokCarouselImagesTotal.Add(float64(len(result.Images)))
	_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "tiktok", "completed", 0, "")
	recordTaskSuccess(queue.TypeTikTokCarousel, "tiktok", start, 0)
	return nil
}
