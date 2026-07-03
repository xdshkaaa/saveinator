package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/locale"
	"saveinator/internal/metrics"
	"saveinator/internal/queue"
	"saveinator/internal/tiktok"
)

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
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, err))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "tiktok", "failed", 0, err.Error())
		return nil
	}

	caption := buildTikTokCaption(result.Title, result.Author, lang)

	switch result.PostType {
	case tiktok.PostTypeCarousel, tiktok.PostTypeAudioOnly:
		if len(result.Images) == 0 && result.AudioPath == "" {
			_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("tiktok.carousel_empty", lang, nil))
			_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "tiktok", "failed", 0, "carousel empty")
			return nil
		}
		if err := h.sender.SendPhotoAlbum(p.ChatID, result.Images, caption); err != nil {
			slog.Warn("tiktok carousel send failed", "err", err)
			_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, err))
			_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "tiktok", "failed", 0, err.Error())
			return nil
		}
		if result.AudioPath != "" {
			if err := h.sender.SendAudio(p.ChatID, result.AudioPath, result.Title, result.Author, 0); err != nil {
				slog.Warn("tiktok audio send failed", "err", err)
			}
		}
	case tiktok.PostTypeVideo:
		if result.VideoPath == "" {
			_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, errors.New("no video file found")))
			_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "tiktok", "failed", 0, "no video file found")
			return nil
		}
		if sendErr := h.sender.SendFile(p.ChatID, result.VideoPath, result.Title, lang, "tiktok", false); sendErr != nil {
			slog.Warn("tiktok send failed", "err", sendErr)
			_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, sendErr))
			_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "tiktok", "failed", 0, sendErr.Error())
			return nil
		}
	default:
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, errors.New("unsupported tiktok post type")))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "tiktok", "failed", 0, "unsupported tiktok post type")
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
		errMsg := "carousel empty"
		if err != nil {
			errMsg = err.Error()
		}
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "tiktok", "failed", 0, errMsg)
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
