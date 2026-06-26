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
	"saveinator/internal/runtime"
	"saveinator/internal/sender"
	"saveinator/internal/video"
	"saveinator/internal/ytdlp"
	"saveinator/internal/youtube"
)

type Handler struct {
	cfg     *config.Settings
	bot     *telego.Bot
	sender  *sender.Telegram
	db      *db.Store
	redis   *redisx.Client
	runtime *runtime.Store
}

func NewHandler(cfg *config.Settings, bot *telego.Bot, store *db.Store, redis *redisx.Client) *Handler {
	return &Handler{
		cfg:     cfg,
		bot:     bot,
		sender:  sender.New(bot),
		db:      store,
		redis:   redis,
		runtime: runtime.NewStore(redis, cfg),
	}
}

func (h *Handler) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(queue.TypeDownload, h.handleDownload)
	mux.HandleFunc(queue.TypeTikTok, h.handleTikTok)
	mux.HandleFunc(queue.TypePinterest, h.handlePinterest)
	mux.HandleFunc(queue.TypeSpotify, h.handleSpotify)
	mux.HandleFunc(queue.TypeSoundCloud, h.handleSoundCloud)
	mux.HandleFunc(queue.TypeBroadcast, h.handleBroadcast)
}

func (h *Handler) handleDownload(ctx context.Context, t *asynq.Task) error {
	var p queue.DownloadPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}
	defer h.releaseLock(ctx, p)
	if h.checkCancelled(ctx, p) {
		return nil
	}
	if p.Platform == "youtube" && p.Quality > 0 && p.AspectRatio != "" {
		return h.runYouTubeDownload(ctx, p)
	}
	return h.runDownload(ctx, p)
}

func (h *Handler) handleTikTok(ctx context.Context, t *asynq.Task) error {
	var p queue.DownloadPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}
	p.Platform = "tiktok"
	defer h.releaseLock(ctx, p)
	if h.checkCancelled(ctx, p) {
		return nil
	}
	return h.runTikTok(ctx, p)
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
		lang = "en"
	}
	_ = h.sender.EditMessageMarkup(p.ChatID, p.MessageID, locale.Get("download.cancelled", lang, nil), nil)
	return true
}

func (h *Handler) releaseLock(ctx context.Context, p queue.DownloadPayload) {
	if p.LockToken == "" || p.LockScene == "" {
		return
	}
	_ = h.redis.ReleaseUserLock(ctx, p.UserID, p.LockScene, p.LockToken)
}

func (h *Handler) runYouTubeDownload(ctx context.Context, p queue.DownloadPayload) error {
	lang := p.Lang
	if lang == "" {
		lang = "en"
	}

	taskDir, err := os.MkdirTemp("", "saveinator-yt-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(taskDir)

	timeout := time.Duration(h.cfg.YouTubeDownloadTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	dlCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	format := p.FormatID
	if format == "" {
		format = youtube.BuildFormat(p.Quality, p.AspectRatio)
	}

	if err := ytdlp.Download(dlCtx, p.URL, taskDir, ytdlp.Options{
		FormatID: format,
		Platform: "youtube",
		Timeout:  timeout,
	}); err != nil {
		slog.Warn("youtube download failed", "url", p.URL, "err", err)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("download.timeout", lang, nil))
		return nil
	}

	files, err := ytdlp.FindMediaFiles(taskDir)
	if err != nil {
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("youtube.process_failed", lang, nil))
		return nil
	}
	sourceVideo := ytdlp.LargestVideo(files)
	if sourceVideo == "" {
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("youtube.process_failed", lang, nil))
		return nil
	}

	processed, err := video.ApplyAspectRatio(dlCtx, sourceVideo, p.AspectRatio, p.Quality)
	if err != nil {
		slog.Warn("youtube transcode failed", "err", err)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("youtube.process_failed", lang, nil))
		return nil
	}

	return h.sendVideoResult(ctx, p, processed, lang)
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
	sourceVideo := ytdlp.LargestVideo(files)
	if sourceVideo == "" && len(images) > 0 {
		caption := locale.Get("download.via_bot", lang, map[string]string{"bot_username": "saveinator_bot"})
		if err := h.sender.SendPhotoAlbum(p.ChatID, images, caption); err != nil {
			slog.Warn("send album failed", "err", err)
		}
		_ = h.sender.DeleteMessage(p.ChatID, p.MessageID)
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, p.Platform, "completed", 0, "")
		return nil
	}

	if sourceVideo == "" {
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("errors.generic", lang, nil))
		return nil
	}

	return h.sendVideoResult(ctx, p, sourceVideo, lang)
}

func (h *Handler) sendVideoResult(ctx context.Context, p queue.DownloadPayload, videoPath, lang string) error {
	sizeMB := float64(fileSize(videoPath)) / (1024 * 1024)
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

	title := filepath.Base(videoPath)
	animation := p.Platform == "x" && !ytdlp.HasAudioStream(videoPath)
	if err := h.sender.SendFile(p.ChatID, videoPath, title, lang, p.Platform, animation); err != nil {
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
