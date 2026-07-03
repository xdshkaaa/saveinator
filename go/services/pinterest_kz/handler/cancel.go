package handler

import (
	"context"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/cancel"
	"saveinator/internal/locale"
)

func (b *Bot) onCancelDownload(bot *telego.Bot) func(context.Context, *telego.Bot, telego.CallbackQuery) {
	return func(ctx context.Context, _ *telego.Bot, query telego.CallbackQuery) {
		lang := b.userLang(ctx, query.From.ID)
		data, ok := cancel.ParseCancel(query.Data)
		if !ok {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText(locale.Get("download.cancel_unavailable", lang, nil)).WithShowAlert())
			return
		}
		if query.From.ID != data.UserID {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText(locale.Get("download.cancel_not_allowed", lang, nil)).WithShowAlert())
			return
		}

		_ = b.redis.SetDownloadCancelled(ctx, data.Scenario, data.UserID, data.Token, 2*time.Hour)
		_ = b.redis.ReleaseUserLock(ctx, data.UserID, data.Scenario, data.Token)

		if query.Message != nil {
			chat := query.Message.GetChat()
			_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
				ChatID:    tu.ID(chat.ID),
				MessageID: query.Message.GetMessageID(),
				Text:      locale.Get("download.cancelled", lang, nil),
			})
		}
		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText(locale.Get("download.cancel_done", lang, nil)))
	}
}

func (b *Bot) onDownloadQueue(bot *telego.Bot) func(context.Context, *telego.Bot, telego.CallbackQuery) {
	return func(ctx context.Context, _ *telego.Bot, query telego.CallbackQuery) {
		lang := b.userLang(ctx, query.From.ID)
		userID, ok := cancel.ParseQueue(query.Data)
		if !ok {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText(locale.Get("download.queue_unavailable", lang, nil)).WithShowAlert())
			return
		}
		if query.From.ID != userID {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText(locale.Get("download.queue_not_allowed", lang, nil)).WithShowAlert())
			return
		}

		active, err := b.redis.GetActiveDownload(ctx, userID)
		if err != nil || active == nil {
			if query.Message != nil {
				chat := query.Message.GetChat()
				_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
					ChatID:    tu.ID(chat.ID),
					MessageID: query.Message.GetMessageID(),
					Text:      locale.Get("download.queue_empty", lang, nil),
				})
			}
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			return
		}

		if query.Message != nil {
			chat := query.Message.GetChat()
			_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
				ChatID:      tu.ID(chat.ID),
				MessageID:   query.Message.GetMessageID(),
				Text:        locale.Get("download.queue_title", lang, nil),
				ReplyMarkup: cancel.ActiveQueueKeyboard(lang, active.Scenario, active.UserID, active.Token),
			})
		}
		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
	}
}
