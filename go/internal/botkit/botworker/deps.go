package botworker

import (
	"context"
	"strings"
	"time"

	"github.com/mymmrac/telego"

	"saveinator/internal/config"
	"saveinator/internal/db"
	"saveinator/internal/locale"
	"saveinator/internal/metrics"
	"saveinator/internal/queue"
	"saveinator/internal/redisx"
	"saveinator/internal/runtime"
)

const maxAdminDebugLen = 3500

// MessageSender is the superset of send operations platforms may need.
type MessageSender interface {
	EditMessage(chatID int64, messageID int, text string) error
	EditMessageMarkup(chatID int64, messageID int, text string, markup *telego.InlineKeyboardMarkup) error
	DeleteMessage(chatID int64, messageID int) error
	SendFile(chatID int64, path, title, lang, platform string, animation bool) error
	SendAudio(chatID int64, path, title, performer string, durationSec int) error
}

// Deps carries everything a platform worker handler needs.
type Deps struct {
	Cfg     *config.Settings
	Sender  MessageSender
	DB      *db.Store
	Redis   *redisx.Client
	Runtime *runtime.Store
}

func NewDeps(cfg *config.Settings, snd MessageSender, store *db.Store, redis *redisx.Client) *Deps {
	return &Deps{
		Cfg:     cfg,
		Sender:  snd,
		DB:      store,
		Redis:   redis,
		Runtime: runtime.NewStore(redis, cfg),
	}
}

func (d *Deps) ReleaseLock(ctx context.Context, userID int64, scene, token string) {
	if token == "" || scene == "" {
		return
	}
	_ = d.Redis.ReleaseUserLock(ctx, userID, scene, token)
}

// CheckCancelled reports whether the download was cancelled and, if so,
// replaces the status message.
func (d *Deps) CheckCancelled(ctx context.Context, p queue.DownloadPayload) bool {
	if p.LockToken == "" || p.LockScene == "" {
		return false
	}
	cancelled, err := d.Redis.IsDownloadCancelled(ctx, p.LockScene, p.UserID, p.LockToken)
	if err != nil || !cancelled {
		return false
	}
	lang := p.Lang
	if lang == "" {
		lang = "en"
	}
	_ = d.Sender.EditMessageMarkup(p.ChatID, p.MessageID, locale.Get("download.cancelled", lang, nil), nil)
	return true
}

func (d *Deps) IsAdmin(userID int64) bool {
	return d.Cfg.AdminTelegramID != 0 && userID == d.Cfg.AdminTelegramID
}

// UserFacingError returns the generic error text, with debug detail appended
// for the admin.
func (d *Deps) UserFacingError(lang string, userID int64, err error) string {
	base := locale.Get("errors.generic", lang, nil)
	if !d.IsAdmin(userID) || err == nil {
		return base
	}
	detail := strings.TrimSpace(err.Error())
	if detail == "" {
		return base
	}
	if len(detail) > maxAdminDebugLen {
		detail = detail[:maxAdminDebugLen] + "…"
	}
	return base + locale.Get("errors.admin_debug", lang, map[string]string{"detail": detail})
}

func RecordTaskSuccess(taskType, platform string, start time.Time, fileBytes int64) {
	metrics.RecordCeleryTask(taskType, "SUCCESS")
	if platform != "" {
		metrics.ObserveDownloadDuration(platform, time.Since(start))
		if fileBytes > 0 {
			metrics.ObserveDownloadFileSize(platform, fileBytes)
		}
	}
}

func RecordTaskFailure(taskType string) {
	metrics.RecordCeleryTask(taskType, "FAILURE")
}
