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

	"saveinator/internal/audio"
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
	"saveinator/internal/translate"
	"saveinator/internal/video"
	"saveinator/internal/x"
	"saveinator/internal/xphotos"
	"saveinator/internal/youtube"
	"saveinator/internal/ytdlp"
)

type Handler struct {
	cfg        *config.Settings
	bot        *telego.Bot
	sender     messageSender
	db         *db.Store
	redis      *redisx.Client
	runtime    *runtime.Store
	ttSessions *tiktok.SessionStore
	translator *translate.Google
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
		translator: translate.NewGoogle(),
	}
}

func (h *Handler) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(queue.TypeDownload, h.handleDownload)
	mux.HandleFunc(queue.TypeTikTok, h.handleTikTok)
	mux.HandleFunc(queue.TypePinterest, h.handlePinterest)
	mux.HandleFunc(queue.TypeSpotify, h.handleSpotify)
	mux.HandleFunc(queue.TypeSoundCloud, h.handleSoundCloud)
	mux.HandleFunc(queue.TypeYandexMusic, h.handleYandexMusic)
	mux.HandleFunc(queue.TypeBroadcast, h.handleBroadcast)
	mux.HandleFunc(queue.TypeTikTokCarousel, h.handleTikTokCarousel)
	mux.HandleFunc(queue.TypeInstagram, h.handleInstagram)
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
	if p.Platform == "youtube" && (p.Quality > 0 || p.AudioOnly) {
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

	if p.AudioOnly {
		return h.runYouTubeAudio(ctx, dlCtx, p, lang, taskDir, start)
	}

	opts := h.ytdlpOpts("youtube", youtube.FormatSelector(p.FormatID, p.Quality, p.AspectRatio), timeout)
	if p.IsTrimmed() {
		opts.DownloadSections = youtube.DownloadSection(p.TrimStart, p.TrimEnd)
	}
	if err := h.downloadYouTubeVideo(dlCtx, p, taskDir, opts); err != nil {
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
	// An empty aspect ratio means "keep the original frame", which skips the
	// re-encode entirely — the common path now that the ratio is opt-in.
	if p.AspectRatio != "" && h.runtime.CurrentBool(ctx, "youtube.transcode_enabled", h.cfg.YouTubeTranscodeEnabled) {
		var transcodeErr error
		processed, transcodeErr = video.ApplyAspectRatio(dlCtx, sourceVideo, p.AspectRatio, p.Quality)
		if transcodeErr != nil {
			slog.Warn("youtube transcode failed", "err", transcodeErr)
			_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, transcodeErr))
			_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "youtube", "failed", 0, transcodeErr.Error())
			return nil
		}
	}

	if h.runtime.CurrentBool(ctx, "youtube.compress_long_enabled", h.cfg.YouTubeCompressLongEnabled) {
		minDuration := h.runtime.CurrentInt(ctx, "youtube.compress_min_duration_sec", h.cfg.YouTubeCompressMinDurationSec)
		if compressed, compressErr := video.CompressIfOversized(dlCtx, processed, minDuration); compressErr == nil {
			processed = compressed
		}
	}

	processed = h.ensureFitsUpload(dlCtx, processed)

	return h.sendVideoResult(ctx, p, processed, lang, queue.TypeDownload, start)
}

// ensureFitsUpload brings a downloaded YouTube video under the effective send
// limit with a targeted re-encode. api.telegram.org rejects any bot upload
// above that limit regardless of method, so without this pass a heavy 1080p
// download completes and then dies at send time. When even the re-encode
// cannot fit, the file goes back unchanged and sendVideoResult refuses it as
// too large against the same effective limit.
func (h *Handler) ensureFitsUpload(ctx context.Context, path string) string {
	limitMB := h.maxFileMB(ctx, "youtube")
	if limitMB <= 0 {
		return path
	}
	sizeMB := float64(fileSize(path)) / (1024 * 1024)
	if sizeMB <= float64(limitMB) {
		return path
	}

	// A small margin keeps multipart framing and mux variance under the cap.
	maxBytes := int64(float64(limitMB) * 1024 * 1024 * 0.98)
	fitted, ok := video.CompressToFit(ctx, path, maxBytes)
	if !ok {
		slog.Warn("youtube fit-to-limit compression failed", "size_mb", sizeMB, "limit_mb", limitMB)
		return path
	}
	slog.Info("youtube video compressed to fit upload limit",
		"before_mb", fmt.Sprintf("%.1f", sizeMB),
		"after_mb", fmt.Sprintf("%.1f", float64(fileSize(fitted))/(1024*1024)))
	return fitted
}

// retryExtractionDelay spaces out the second attempt below. Long enough that
// the retry is a genuinely new extraction rather than part of the same refused
// burst, short enough that the user keeps staring at one "downloading" message.
const retryExtractionDelay = 2 * time.Second

// downloadYouTubeVideo runs the download, and on a format failure retries once
// with the generic selector.
//
// The point of the retry is the fresh extraction, not the different selector:
// which formats exist at all depends on which player client answered, and the
// one client that serves them without a PO token is refused intermittently. A
// second attempt often draws a working answer. When the refusal is not
// momentary both attempts fail identically and the caller reports it.
func (h *Handler) downloadYouTubeVideo(dlCtx context.Context, p queue.DownloadPayload, taskDir string, opts ytdlp.Options) error {
	err := ytdlp.Download(dlCtx, p.URL, taskDir, opts)
	if err == nil || !ytdlp.IsFormatUnavailableError(err) {
		return err
	}

	slog.Warn("youtube formats unavailable, retrying extraction", "url", p.URL, "err", err)
	select {
	case <-dlCtx.Done():
		return err
	case <-time.After(retryExtractionDelay):
	}

	retryOpts := opts
	retryOpts.FormatID = youtube.BuildFormat(p.Quality, p.AspectRatio)
	return ytdlp.Download(dlCtx, p.URL, taskDir, retryOpts)
}

// runYouTubeAudio serves the Mp3 button: the soundtrack only, trimmed to the
// selected fragment when there is one.
func (h *Handler) runYouTubeAudio(ctx, dlCtx context.Context, p queue.DownloadPayload, lang, taskDir string, start time.Time) error {
	section := ""
	if p.IsTrimmed() {
		section = youtube.DownloadSection(p.TrimStart, p.TrimEnd)
	}

	path, err := audio.DownloadYouTubeAudio(dlCtx, p.URL, taskDir, "mp3", section)
	if err != nil {
		slog.Warn("youtube audio download failed", "url", p.URL, "err", err)
		metrics.RecordYtdlpError("youtube")
		recordTaskFailure(queue.TypeDownload)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, err))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "youtube", "failed", 0, err.Error())
		return nil
	}

	sizeMB := float64(fileSize(path)) / (1024 * 1024)
	if limit := float64(h.maxFileMB(ctx, "youtube")); sizeMB > limit {
		msg := locale.Get("download.too_large", lang, map[string]string{
			"size":  fmt.Sprintf("%.1f", sizeMB),
			"limit": fmt.Sprintf("%.0f", limit),
		})
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, msg)
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "youtube", "failed", sizeMB, "too large")
		return nil
	}

	title := p.Title
	if title == "" {
		title = youtube.DisplayTitle(path)
	}
	if err := h.sender.SendAudio(p.ChatID, path, title, p.Author, 0, ""); err != nil {
		slog.Warn("send audio failed", "err", err)
		recordTaskFailure(queue.TypeDownload)
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, h.userFacingError(lang, p.UserID, err))
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "youtube", "failed", sizeMB, err.Error())
		return nil
	}

	_ = h.sender.DeleteMessage(p.ChatID, p.MessageID)
	_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "youtube", "completed", sizeMB, "")
	recordTaskSuccess(queue.TypeDownload, "youtube", start, fileSize(path))
	return nil
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

	caption := h.buildXPhotoCaption(ctx, statusID, result, lang)
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

func (h *Handler) buildXPhotoCaption(ctx context.Context, statusID string, result *xphotos.Result, lang string) string {
	title := ""
	if result != nil {
		title = x.CleanRawTitle(result.Title)
	}
	if title == "" && statusID != "" {
		title = x.ResolveTitle(ctx, statusID, "")
	}
	if title == "" {
		return buildMediaCaption(title, lang)
	}
	return buildMediaCaption(h.translateXTitle(ctx, title), lang)
}

// translateXTitle appends a Russian auto-translation under the original post
// text when the runtime switch x.auto_translate is on and the text is not
// already Russian. Translation failures fall back to the original text.
func (h *Handler) translateXTitle(ctx context.Context, title string) string {
	if !h.runtime.CurrentBool(ctx, "x.auto_translate", true) {
		return title
	}
	translated := h.translator.Text(ctx, title)
	if translated == title {
		return title
	}
	return title + "\n\n" + translated
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
		title = h.translateXTitle(ctx, x.ResolveTitle(ctx, statusID, videoPath))
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

// maxFileMB is the effective per-platform send ceiling: the platform's own
// cap clamped by the Telegram bot upload limit, because api.telegram.org
// refuses anything larger no matter which send method is used.
func (h *Handler) maxFileMB(ctx context.Context, platform string) int {
	limit := h.runtime.PlatformMaxFileMB(ctx, platform)
	upload := h.runtime.CurrentInt(ctx, "global.telegram_upload_limit_mb", h.cfg.TelegramUploadLimitMB)
	if upload > 0 && limit > upload {
		return upload
	}
	return limit
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

	if len(result.Items) > 1 {
		return h.sendPinterestItems(ctx, p, result.Items, lang, start)
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

// sendPinterestItems delivers every item from a board/multi-media pin instead
// of picking a single one: images are grouped into an album and videos are
// sent individually, skipping any item that exceeds its size limit.
func (h *Handler) sendPinterestItems(ctx context.Context, p queue.DownloadPayload, items []pinterest.MediaItem, lang string, start time.Time) error {
	imageLimit := float64(h.runtime.CurrentInt(ctx, "global.document_limit_mb", h.cfg.SendDocumentLimitMB))
	videoLimit := float64(h.runtime.PlatformMaxFileMB(ctx, "pinterest"))

	var imagePaths []string
	var totalSize int64
	sentAny := false

	for _, item := range items {
		if _, err := os.Stat(item.FilePath); err != nil {
			slog.Warn("pinterest media file missing", "path", item.FilePath, "err", err)
			continue
		}
		sizeMB := float64(item.FileSize) / (1024 * 1024)
		if item.MediaType == "video" {
			if sizeMB > videoLimit {
				continue
			}
			title := item.Title
			if title == "" {
				title = pinterest.DisplayTitle(item.FilePath)
			}
			if err := h.sender.SendFile(p.ChatID, item.FilePath, title, lang, "pinterest", false); err != nil {
				slog.Warn("pinterest send failed", "err", err)
				continue
			}
			totalSize += item.FileSize
			sentAny = true
			continue
		}
		if sizeMB > imageLimit {
			continue
		}
		imagePaths = append(imagePaths, item.FilePath)
		totalSize += item.FileSize
	}

	if len(imagePaths) > 0 {
		caption := buildMediaCaption("", lang)
		if err := h.sender.SendPhotoAlbum(p.ChatID, imagePaths, caption); err != nil {
			slog.Warn("pinterest album send failed", "err", err)
		} else {
			sentAny = true
		}
	}

	if !sentAny {
		_ = h.sender.EditMessage(p.ChatID, p.MessageID, locale.Get("pinterest.all_too_large", lang, nil))
		recordTaskFailure(queue.TypePinterest)
		_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "pinterest", "failed", 0, "all items too large or missing")
		return nil
	}

	_ = h.sender.DeleteMessage(p.ChatID, p.MessageID)
	_ = h.db.RecordDownload(ctx, p.UserID, p.ChatID, p.URL, "pinterest", "completed", float64(totalSize)/(1024*1024), "")
	recordTaskSuccess(queue.TypePinterest, "pinterest", start, totalSize)
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
