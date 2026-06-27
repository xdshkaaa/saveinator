package handler

import (
	"context"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/locale"
	"saveinator/internal/metrics"
	"saveinator/internal/queue"
	"saveinator/internal/tiktok"
)

func (b *Bot) onTikTokCarousel(bot *telego.Bot) func(context.Context, *telego.Bot, telego.CallbackQuery) {
	return func(ctx context.Context, _ *telego.Bot, query telego.CallbackQuery) {
		lang := b.userLang(ctx, query.From.ID)
		userID, token, ok := tiktok.ParseCarouselCallback(query.Data)
		if !ok {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			return
		}
		if query.From.ID != userID {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText(locale.Get("download.cancel_not_allowed", lang, nil)).WithShowAlert())
			return
		}

		session, err := b.ttSessions.Get(ctx, token)
		if err != nil || session == nil || session.UserID != userID {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText(locale.Get("tiktok.carousel_images_unavailable", lang, nil)).WithShowAlert())
			return
		}

		lockToken, acquired, err := b.redis.AcquireUserLock(ctx, userID, "tiktok", lockTTL(b.cfg, "tiktok"))
		if err != nil || !acquired {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText(locale.Get("errors.busy", lang, nil)).WithShowAlert())
			return
		}

		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText(locale.Get("tiktok.carousel_images_downloading", lang, nil)))

		if query.Message != nil {
			_, _ = bot.EditMessageReplyMarkup(&telego.EditMessageReplyMarkupParams{
				ChatID:    tu.ID(query.Message.GetChat().ID),
				MessageID: query.Message.GetMessageID(),
			})
		}

		_ = b.q.EnqueueTikTokCarousel(queue.DownloadPayload{
			URL:          session.URL,
			Platform:     "tiktok",
			ChatID:       session.ChatID,
			UserID:       userID,
			Lang:         session.Lang,
			LockToken:    lockToken,
			LockScene:    "tiktok",
			SessionToken: token,
		})
		metrics.DownloadsEnqueued.WithLabelValues("tiktok").Inc()
	}
}
