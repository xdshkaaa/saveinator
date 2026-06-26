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

	"saveinator/internal/config"
	"saveinator/internal/db"
	"saveinator/internal/locale"
	"saveinator/internal/queue"
	"saveinator/internal/redisx"
	"saveinator/internal/sender"
	"saveinator/internal/ytdlp"
)

type Handler struct {
	cfg    *config.Settings
	bot    *telego.Bot
	sender *sender.Telegram
	db     *db.Store
	redis  *redisx.Client
}

func NewHandler(cfg *config.Settings, bot *telego.Bot, store *db.Store, redis *redisx.Client) *Handler {
	return &Handler{
		cfg:    cfg,
		bot:    bot,
		sender: sender.New(bot),
		db:     store,
		redis:  redis,
	}
}

func (h *Handler) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(queue.TypeDownload, h.handleDownload)
	mux.HandleFunc(queue.TypeTikTok, h.handleTikTok)
}

func (h *Handler) handleDownload(ctx context.Context, t *asynq.Task) error {
	var p queue.DownloadPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}
	defer h.releaseLock(ctx, p)
	return h.runDownload(ctx, p)
}

func (h *Handler) handleTikTok(ctx context.Context, t *asynq.Task) error {
	var p queue.DownloadPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}
	p.Platform = "tiktok"
	defer h.releaseLock(ctx, p)
	return h.runDownload(ctx, p)
}

func (h *Handler) releaseLock(ctx context.Context, p queue.DownloadPayload) {
	if p.LockToken == "" || p.LockScene == "" {
		return
	}
	_ = h.redis.ReleaseUserLock(ctx, p.UserID, p.LockScene, p.LockToken)
}

func (h *Handler) runDownload(ctx context.Context, p queue.DownloadPayload) error {
	lang := p.Lang
	if lang == "" {
		lang = "en"
	}

	_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("download.downloading", lang, nil))

	taskDir, err := os.MkdirTemp("", "saveinator-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(taskDir)

	timeout := time.Duration(h.cfg.DownloadTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	dlCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	format := p.FormatID
	if format == "" {
		format = "best"
	}

	err = ytdlp.Download(dlCtx, p.URL, taskDir, ytdlp.Options{
		FormatID:         format,
		Platform:         p.Platform,
		InstagramCookies: h.cfg.InstagramCookiesPath,
		TikTokCookies:    h.cfg.TikTokCookiesPath,
		Timeout:          timeout,
	})
	if err != nil {
		slog.Warn("download failed", "url", p.URL, "err", err)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("download.timeout", lang, nil))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, p.Platform, "failed", 0, err.Error())
		return nil
	}

	files, err := ytdlp.FindMediaFiles(taskDir)
	if err != nil || len(files) == 0 {
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("errors.generic", lang, nil))
		return nil
	}

	images := ytdlp.ImageFiles(files)
	video := ytdlp.LargestVideo(files)
	if video == "" && len(images) > 0 {
		caption := locale.Get("download.via_bot", lang, map[string]string{"bot_username": "saveinator_bot"})
		if err := h.sender.SendPhotoAlbum(p.ChatID, images, caption); err != nil {
			slog.Warn("send album failed", "err", err)
		}
		_ = h.sender.DeleteMessage(p.ChatID, p.MessageID)
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, p.Platform, "completed", 0, "")
		return nil
	}

	if video == "" {
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("errors.generic", lang, nil))
		return nil
	}

	sizeMB := float64(fileSize(video)) / (1024 * 1024)
	limit := float64(h.maxFileMB(p.Platform))
	if sizeMB > limit {
		msg := locale.Get("download.too_large", lang, map[string]string{
			"size":  fmt.Sprintf("%.1f", sizeMB),
			"limit": fmt.Sprintf("%.0f", limit),
		})
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, msg)
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, p.Platform, "failed", sizeMB, "too large")
		return nil
	}

	title := filepath.Base(video)
	animation := p.Platform == "x" && !ytdlp.HasAudioStream(video)
	if err := h.sender.SendFile(p.ChatID, video, title, lang, p.Platform, animation); err != nil {
		slog.Warn("send file failed", "err", err)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("errors.generic", lang, nil))
		return nil
	}

	_ = h.sender.DeleteMessage(p.ChatID, p.MessageID)
	_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, p.Platform, "completed", sizeMB, "")
	return nil
}

func (h *Handler) maxFileMB(platform string) int {
	switch platform {
	case "youtube":
		return h.cfg.YouTubeMaxFileSizeMB
	default:
		return h.cfg.SendVideoLimitMB
	}
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
