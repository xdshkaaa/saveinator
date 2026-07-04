package botkit

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/mymmrac/telego"

	"saveinator/internal/botkit/botworker"
	"saveinator/internal/linkparser"
)

// Platform plugs a downloader into a bot: link matching + handler-side
// dispatch on the bot process, and task handling on the worker side.
type Platform interface {
	// Slug is the platform name used in metrics and runtime toggles.
	Slug() string
	// Match reports whether this platform serves the parsed link.
	Match(link linkparser.ParsedLink) bool
	// HandleLink processes a matched link on the bot side (enqueue, reply…).
	HandleLink(ctx context.Context, b *Bot, tg *telego.Bot, msg telego.Message, lang string, link linkparser.ParsedLink, batch bool)
	// RegisterWorker registers the platform's asynq task handlers.
	RegisterWorker(mux *asynq.ServeMux, d *botworker.Deps, bc *BotConfig)
}
