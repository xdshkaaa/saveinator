package handler

import (
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"

	"saveinator/internal/metrics"
)

func metricsMiddleware() th.Middleware {
	return func(bot *telego.Bot, update telego.Update, next th.Handler) {
		metrics.RecordUpdate(update)
		next(bot, update)
	}
}
