package handler

import (
	"context"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"

	"saveinator/internal/metrics"
	"saveinator/internal/redisx"
)

func metricsMiddleware(redis *redisx.Client) th.Middleware {
	return func(bot *telego.Bot, update telego.Update, next th.Handler) {
		metrics.RecordUpdate(update)
		if userID := activeUserID(update); userID != 0 {
			_ = redis.TouchActiveUser(context.Background(), userID)
		}
		next(bot, update)
	}
}

func activeUserID(update telego.Update) int64 {
	switch {
	case update.Message != nil && update.Message.From != nil:
		return update.Message.From.ID
	case update.EditedMessage != nil && update.EditedMessage.From != nil:
		return update.EditedMessage.From.ID
	case update.CallbackQuery != nil:
		return update.CallbackQuery.From.ID
	default:
		return 0
	}
}
