package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/mymmrac/telego"

	"saveinator/internal/config"
	"saveinator/internal/db"
	"saveinator/internal/locale"
	"saveinator/internal/metrics"
	"saveinator/internal/pinterest"
	"saveinator/internal/queue"
	"saveinator/internal/redisx"
	"saveinator/internal/runtime"
	"saveinator/internal/sender"
	"saveinator/internal/tiktok"
	"saveinator/internal/video"
	"saveinator/internal/x"
	"saveinator/internal/xphotos"
	"saveinator/internal/ytdlp"
	"saveinator/internal/youtube"
)

type Handler struct {
	cfg        *config.Settings
	bot        *telego.Bot
	sender     messageSender
	db         *db.Store
	redis      *redisx.Client
	runtime    *runtime.Store
	ttSessions *tiktok.SessionStore
}

func NewHandler(cfg *config.Settings, bot *telego.Bot, store *db.Store, redis *redisx.Client) *Handler {
	return &Handler{
		cfg:        cfg,
		bot:        bot,
		sender:     sender.New(bot),
		db:         store,
		redis:      redis,
		runtime:    runtime.NewStore(redis, cfg),
		ttSessions: tiktok.NewSessionStore(redis.Raw()),
	}
}

func (h *Handler) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(queue.TypeDownload, h.handleDownload)
	mux.HandleFunc(queue.TypeTikTok, h.handleTikTok)
	mux.HandleFunc(queue.TypePinterest, h.handlePinterest)
	mux.HandleFunc(queue.TypeSpotify, h.handleSpotify)
	mux.HandleFunc(queue.TypeSoundCloud, h.handleSoundCloud)
	mux.HandleFunc(queue.TypeBroadcast, h.handleBroadcast)
	mux.HandleFunc(queue.TypeTikTokCarousel, h.handleTikTokCarousel)
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

func (h *Handler) handleTikTokCarousel(ctx context.Context, t *asynq.Task) error {
	var p queue.DownloadPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}
	defer h.releaseLock(ctx, p)
	return h.runTikTokCarouselImages(ctx, p)
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
	start := time.Now()
	lang := p.Lang
	if lang == "" {
		lang = "en"
	}

	taskDir, err := os.MkdirTemp("", "saveinator-yt-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(taskDir)

	timeout := time.Duration(h.runtime.PlatformTimeoutSec(ctx, "youtube")) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	dlCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	format := p.FormatID
	if format == "" {
		format = youtube.BuildFormat(p.Quality, p.AspectRatio)
	}

	if err := ytdlp.Download(dlCtx, p.URL, taskDir, h.ytdlpOpts("youtube", format, timeout)); err != nil {
		slog.Warn("youtube download failed", "url", p.URL, "err", err)
		metrics.RecordYtdlpError("youtube")
		recordTaskFailure(queue.TypeDownload)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, err))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "youtube", "failed", 0, err.Error())
		return nil
	}

	files, err := ytdlp.FindMediaFiles(taskDir)
	if err != nil {
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, err))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "youtube", "failed", 0, err.Error())
		return nil
	}
	sourceVideo := ytdlp.LargestVideo(files)
	if sourceVideo == "" {
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, errors.New("no video file found")))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "youtube", "failed", 0, "no video file found")
		return nil
	}

	processed := sourceVideo
	if h.runtime.CurrentBool(ctx, "youtube.transcode_enabled", h.cfg.YouTubeTranscodeEnabled) {
		var transcodeErr error
		processed, transcodeErr = video.ApplyAspectRatio(dlCtx, sourceVideo, p.AspectRatio, p.Quality)
		if transcodeErr != nil {
			slog.Warn("youtube transcode failed", "err", transcodeErr)
			_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, transcodeErr))
			_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "youtube", "failed", 0, transcodeErr.Error())
			return nil
		}
	}

	return h.sendVideoResult(ctx, p, processed, lang, queue.TypeDownload, start)
}

func (h *Handler) runDownload(ctx context.Context, p queue.DownloadPayload) error {
	start := time.Now()
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

	timeout := time.Duration(h.runtime.PlatformTimeoutSec(ctx, p.Platform)) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	dlCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	format := p.FormatID
	if format == "" {
		format = "best"
	}

	err = ytdlp.Download(dlCtx, p.URL, taskDir, h.ytdlpOpts(p.Platform, format, timeout))
	if err != nil {
		if p.Platform == "x" {
			return h.runXPhotos(ctx, p, lang, taskDir, queue.TypeDownload, start)
		}
		slog.Warn("download failed", "url", p.URL, "platform", p.Platform, "err", err)
		metrics.RecordYtdlpError(p.Platform)
		recordTaskFailure(queue.TypeDownload)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, err))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, p.Platform, "failed", 0, err.Error())
		return nil
	}

	files, err := ytdlp.FindMediaFiles(taskDir)
	if err != nil || len(files) == 0 {
		if p.Platform == "x" {
			return h.runXPhotos(ctx, p, lang, taskDir, queue.TypeDownload, start)
		}
		recordTaskFailure(queue.TypeDownload)
		failErr := err
		if failErr == nil {
			failErr = errors.New("no media files found")
		}
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, failErr))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, p.Platform, "failed", 0, failErr.Error())
		return nil
	}

	images := ytdlp.ImageFiles(files)
	sourceVideo := ytdlp.LargestVideo(files)
	if sourceVideo == "" && len(images) > 0 {
		caption := buildMediaCaption("", lang)
		if err := h.sender.SendPhotoAlbum(p.ChatID, images, caption); err != nil {
			slog.Warn("send album failed", "err", err)
			recordTaskFailure(queue.TypeDownload)
			_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, p.Platform, "failed", 0, err.Error())
		} else {
			recordTaskSuccess(queue.TypeDownload, p.Platform, start, 0)
			_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, p.Platform, "completed", 0, "")
		}
		_ = h.sender.DeleteMessage(p.ChatID, p.MessageID)
		return nil
	}

	if sourceVideo == "" {
		if p.Platform == "x" {
			return h.runXPhotos(ctx, p, lang, taskDir, queue.TypeDownload, start)
		}
		recordTaskFailure(queue.TypeDownload)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, errors.New("no video file found")))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, p.Platform, "failed", 0, "no video file found")
		return nil
	}

	return h.sendVideoResult(ctx, p, sourceVideo, lang, queue.TypeDownload, start)
}

func (h *Handler) runXPhotos(ctx context.Context, p queue.DownloadPayload, lang, taskDir string, taskType string, start time.Time) error {
	maxItems := h.runtime.CurrentInt(ctx, "x.max_items_per_post", 4)
	statusID := p.XStatusID
	if statusID == "" {
		statusID = xphotos.ExtractStatusID(p.URL)
	}

	result, paths, err := xphotos.DownloadPhotos(ctx, p.URL, taskDir, statusID, maxItems)
	if err != nil {
		slog.Warn("x photo download failed", "url", p.URL, "status_id", statusID, "err", err)
		recordTaskFailure(taskType)
		msg := h.userFacingError(lang, p.UserID, err)
		if errors.Is(err, xphotos.ErrNotFound) {
			msg = locale.Get("x.text_only", lang, nil)
		}
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, msg)
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "x", "failed", 0, err.Error())
		return nil
	}

	caption := buildXPhotoCaption(ctx, statusID, result, lang)
	if err := h.sender.SendPhotoAlbum(p.ChatID, paths, caption); err != nil {
		slog.Warn("x photo album send failed", "err", err)
		recordTaskFailure(taskType)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, err))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "x", "failed", 0, err.Error())
		return nil
	}
	_ = h.sender.DeleteMessage(p.ChatID, p.MessageID)
	_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "x", "completed", 0, "")
	recordTaskSuccess(taskType, "x", start, 0)
	return nil
}

func buildXPhotoCaption(ctx context.Context, statusID string, result *xphotos.Result, lang string) string {
	title := ""
	if result != nil {
		title = x.CleanRawTitle(result.Title)
	}
	if title == "" && statusID != "" {
		title = x.ResolveTitle(ctx, statusID, "")
	}
	return buildMediaCaption(title, lang)
}

func buildMediaCaption(title, lang string) string {
	via := locale.Get("download.via_bot", lang, map[string]string{"bot_username": "saveinator_bot"})
	title = strings.TrimSpace(title)
	if title == "" {
		return via
	}
	return title + "\n\n" + via
}

func (h *Handler) sendVideoResult(ctx context.Context, p queue.DownloadPayload, videoPath, lang, taskType string, start time.Time) error {
	sizeMB := float64(fileSize(videoPath)) / (1024 * 1024)
	limit := float64(h.maxFileMB(ctx, p.Platform))
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
	switch p.Platform {
	case "youtube":
		title = youtube.DisplayTitle(videoPath)
	case "x":
		statusID := p.XStatusID
		if statusID == "" {
			statusID = xphotos.ExtractStatusID(p.URL)
		}
		title = x.ResolveTitle(ctx, statusID, videoPath)
	}
	animation := p.Platform == "x" && !ytdlp.HasAudioStream(videoPath)
	if err := h.sender.SendFile(p.ChatID, videoPath, title, lang, p.Platform, animation); err != nil {
		slog.Warn("send file failed", "err", err)
		recordTaskFailure(taskType)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, err))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, p.Platform, "failed", sizeMB, err.Error())
		return nil
	}

	_ = h.sender.DeleteMessage(p.ChatID, p.MessageID)
	_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, p.Platform, "completed", sizeMB, "")
	recordTaskSuccess(taskType, p.Platform, start, fileSize(videoPath))
	return nil
}

func (h *Handler) maxFileMB(ctx context.Context, platform string) int {
	return h.runtime.PlatformMaxFileMB(ctx, platform)
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func (h *Handler) handlePinterest(ctx context.Context, t *asynq.Task) error {
	var p queue.DownloadPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}
	p.Platform = "pinterest"
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
			_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "pinterest", "failed", 0, "no media")
			return nil
		}
		slog.Warn("pinterest download failed", "err", err)
		recordTaskFailure(queue.TypePinterest)
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
		recordTaskFailure(queue.TypePinterest)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, err))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "pinterest", "failed", sizeMB, err.Error())
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
