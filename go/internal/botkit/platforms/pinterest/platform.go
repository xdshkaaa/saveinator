package pinterestplat

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/hibiken/asynq"
	"github.com/mymmrac/telego"

	"saveinator/internal/botkit"
	"saveinator/internal/botkit/botworker"
	"saveinator/internal/linkparser"
	"saveinator/internal/locale"
	"saveinator/internal/pinterest"
	"saveinator/internal/queue"
)

// Platform implements botkit.Platform for Pinterest downloads.
type Platform struct{}

func New() *Platform { return &Platform{} }

func init() {
	botkit.RegisterPlatform("pinterest", func() botkit.Platform { return New() })
}

func (p *Platform) Slug() string { return "pinterest" }

func (p *Platform) Match(link linkparser.ParsedLink) bool {
	return link.Platform == linkparser.PlatformPinterest
}

func (p *Platform) HandleLink(ctx context.Context, b *botkit.Bot, tg *telego.Bot, msg telego.Message, lang string, link linkparser.ParsedLink, batch bool) {
	b.EnqueueDownload(ctx, tg, msg, lang, link, "pinterest", queue.TypePinterest, batch)
}

func (p *Platform) RegisterWorker(mux *asynq.ServeMux, d *botworker.Deps, bc *botkit.BotConfig) {
	h := &taskHandler{d: d, botID: bc.Slug}
	mux.HandleFunc(queue.TypePinterest, h.handle)
	// Transitional alias: tasks enqueued by the pre-botkit pinterest_kz
	// binary still carry the legacy type.
	mux.HandleFunc(queue.TypePinterestKZ, h.handle)
}

type taskHandler struct {
	d     *botworker.Deps
	botID string
}

func (h *taskHandler) handle(ctx context.Context, t *asynq.Task) error {
	var p queue.DownloadPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}
	defer h.d.ReleaseLock(ctx, p.UserID, p.LockScene, p.LockToken)
	if h.d.CheckCancelled(ctx, p) {
		return nil
	}
	return h.run(ctx, t.Type(), p)
}

func (h *taskHandler) run(ctx context.Context, taskType string, p queue.DownloadPayload) error {
	d := h.d
	start := time.Now()
	lang := p.Lang
	if lang == "" {
		lang = "en"
	}

	_ = d.Sender.EditMessage(p.ChatID, p.MessageID, locale.Get("download.downloading", lang, nil))

	taskDir, err := os.MkdirTemp("", "saveinator-pin-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(taskDir)

	client := pinterest.NewClient(d.Cfg.PinterestCookiesPath, d.Runtime.CurrentInt(ctx, "pinterest.timeout_sec", d.Cfg.PinterestTimeoutSeconds))
	maxItems := d.Runtime.CurrentInt(ctx, "pinterest.max_items_per_board", d.Cfg.PinterestMaxItems)
	downloadImages := d.Runtime.CurrentBool(ctx, "pinterest.download_images", d.Cfg.PinterestDownloadImages)
	downloadVideos := d.Runtime.CurrentBool(ctx, "pinterest.download_videos", d.Cfg.PinterestDownloadVideos)
	result, err := client.Download(ctx, p.URL, taskDir, maxItems, downloadImages, downloadVideos)
	if err != nil {
		if errors.Is(err, pinterest.ErrNoMedia) {
			_ = d.Sender.EditMessage(p.ChatID, p.MessageID, locale.Get("pinterest.no_media", lang, nil))
			botworker.RecordTaskFailure(h.botID, taskType)
			return nil
		}
		slog.Warn("pinterest download failed", "err", err)
		botworker.RecordTaskFailure(h.botID, taskType)
		_ = d.Sender.EditMessage(p.ChatID, p.MessageID, d.UserFacingError(lang, p.UserID, err))
		return nil
	}
	if len(result.Items) == 0 {
		_ = d.Sender.EditMessage(p.ChatID, p.MessageID, locale.Get("pinterest.no_media", lang, nil))
		return nil
	}

	item := pickPinterestItem(result.Items)
	sizeMB := float64(item.FileSize) / (1024 * 1024)
	limit := float64(d.Runtime.PlatformMaxFileMB(ctx, "pinterest"))
	if item.MediaType == "image" {
		limit = float64(d.Runtime.CurrentInt(ctx, "global.document_limit_mb", d.Cfg.SendDocumentLimitMB))
	}
	if sizeMB > limit {
		_ = d.Sender.EditMessage(p.ChatID, p.MessageID, locale.Get("pinterest.all_too_large", lang, nil))
		return nil
	}

	title := item.Title
	if title == "" {
		title = pinterest.DisplayTitle(item.FilePath)
	}
	if _, err := os.Stat(item.FilePath); err != nil {
		slog.Warn("pinterest media file missing", "path", item.FilePath, "err", err)
		_ = d.Sender.EditMessage(p.ChatID, p.MessageID, d.UserFacingError(lang, p.UserID, err))
		return nil
	}
	if err := d.Sender.SendFile(p.ChatID, item.FilePath, title, lang, "pinterest", false); err != nil {
		slog.Warn("pinterest send failed", "err", err)
		botworker.RecordTaskFailure(h.botID, taskType)
		_ = d.Sender.EditMessage(p.ChatID, p.MessageID, d.UserFacingError(lang, p.UserID, err))
		return nil
	}
	_ = d.Sender.DeleteMessage(p.ChatID, p.MessageID)
	_ = d.DB.RecordDownloadForBot(ctx, p.UserID, p.ChatID, p.URL, "pinterest", "completed", sizeMB, "", h.botID)
	botworker.RecordTaskSuccess(h.botID, taskType, "pinterest", start, item.FileSize)
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
