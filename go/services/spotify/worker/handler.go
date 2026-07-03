package worker

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/mymmrac/telego"

	"saveinator/internal/config"
	"saveinator/internal/db"
	"saveinator/internal/queue"
	"saveinator/internal/redisx"
	"saveinator/internal/runtime"
)

type Handler struct {
	cfg     *config.Settings
	sender  messageSender
	db      *db.Store
	redis   *redisx.Client
	runtime *runtime.Store
}

type messageSender interface {
	EditMessage(chatID int64, messageID int, text string) error
	EditMessageMarkup(chatID int64, messageID int, text string, markup *telego.InlineKeyboardMarkup) error
	DeleteMessage(chatID int64, messageID int) error
	SendFile(chatID int64, path, title, lang, platform string, animation bool) error
	SendAudio(chatID int64, path, title, performer string, durationSec int) error
}

func NewHandler(cfg *config.Settings, snd messageSender, store *db.Store, redis *redisx.Client) *Handler {
	return &Handler{
		cfg:     cfg,
		sender:  snd,
		db:      store,
		redis:   redis,
		runtime: runtime.NewStore(redis, cfg),
	}
}

func (h *Handler) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(queue.TypeSpotify, h.handleSpotify)
}

func (h *Handler) releaseMusicUserLock(ctx context.Context, p queue.MusicPayload) {
	if p.LockToken == "" || p.LockScene == "" {
		return
	}
	_ = h.redis.ReleaseUserLock(ctx, p.UserID, p.LockScene, p.LockToken)
}
