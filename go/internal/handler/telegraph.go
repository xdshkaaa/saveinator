package handler

import (
	"context"
	"log/slog"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/locale"
	"saveinator/internal/queue"
	"saveinator/internal/telegraph"
)

// onTelegraphTranslate handles the "translate article to Russian" inline
// button: after an authz check it queues the worker task that builds the RU
// Telegraph page and answers the query with a progress toast.
func (b *Bot) onTelegraphTranslate(bot *telego.Bot) func(context.Context, *telego.Bot, telego.CallbackQuery) {
	return func(ctx context.Context, _ *telego.Bot, query telego.CallbackQuery) {
		lang := b.userLang(ctx, query.From.ID)
		data, ok := telegraph.ParseTranslate(query.Data)
		if !ok {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).
				WithText(locale.Get("errors.callback_invalid", lang, nil)).WithShowAlert())
			return
		}
		if query.From.ID != data.UserID {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).
				WithText(locale.Get("telegraph.not_allowed", lang, nil)).WithShowAlert())
			return
		}

		payload := queue.TelegraphTranslatePayload{
			ThreadID: data.ThreadID,
			UserID:   query.From.ID,
			Lang:     lang,
		}
		if query.Message != nil {
			payload.ChatID = query.Message.GetChat().ID
			payload.MessageID = query.Message.GetMessageID()
		}

		if err := b.q.EnqueueTelegraphTranslate(payload); err != nil {
			slog.Warn("telegraph translate enqueue failed", "err", err)
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).
				WithText(locale.Get("errors.generic", lang, nil)).WithShowAlert())
			return
		}

		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText(locale.Get("telegraph.translating", lang, nil)))
	}
}
