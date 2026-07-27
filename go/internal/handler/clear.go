package handler

import (
	"context"
	"log/slog"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/locale"
	"saveinator/internal/metrics"
	"saveinator/internal/queue"
)

func (b *Bot) onClear(bot *telego.Bot) func(context.Context, *telego.Bot, telego.Message) {
	return func(ctx context.Context, _ *telego.Bot, msg telego.Message) {
		if msg.From == nil {
			return
		}
		metrics.RecordCommand("clear")

		lang := b.userLang(ctx, msg.From.ID)
		userID := msg.From.ID
		cleared := false

		active, err := b.redis.GetActiveDownload(ctx, userID)
		if err == nil && active != nil {
			_ = b.redis.SetDownloadCancelled(ctx, active.Scenario, userID, active.Token, 2*time.Hour)
			cleared = true
		}
		if err := b.redis.ForceReleaseUserLock(ctx, userID); err != nil {
			slog.Warn("clear force release lock failed", "user", userID, "err", err)
		} else if active != nil {
			cleared = true
		}

		insp, err := queue.NewInspector(b.cfg.RedisURL)
		if err != nil {
			slog.Warn("clear queue inspector failed", "err", err)
			_, _ = bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), locale.Get("errors.generic", lang, nil)))
			return
		}
		defer insp.Close()

		result, lockRefs, err := queue.ClearUserTasks(insp, userID)
		if err != nil {
			slog.Warn("clear queue tasks failed", "user", userID, "err", err)
			_, _ = bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), locale.Get("errors.generic", lang, nil)))
			return
		}

		for _, ref := range lockRefs {
			_ = b.redis.SetDownloadCancelled(ctx, ref.Scene, userID, ref.Token, 2*time.Hour)
		}
		if result.Any() {
			cleared = true
		}

		text := locale.Get("clear.success", lang, nil)
		if !cleared {
			text = locale.Get("clear.empty", lang, nil)
		}
		_, _ = bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), text))
	}
}
