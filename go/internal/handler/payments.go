package handler

import (
	"context"
	"log/slog"
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/locale"
	"saveinator/internal/metrics"
)

// The watermark (the "via @bot" caption footer) is disabled per user by a
// one-time Telegram Stars purchase; admins toggle it for free.
const (
	watermarkProduct    = "no_watermark"
	watermarkPriceStars = 10
)

// wmEntitled reports whether the user may use the watermark toggle: admins
// get it for free, everyone else needs a recorded purchase.
func (b *Bot) wmEntitled(ctx context.Context, userID int64) bool {
	if b.isAdmin(userID) {
		return true
	}
	paid, err := b.db.HasPurchase(ctx, userID, watermarkProduct)
	if err != nil {
		slog.Warn("purchase lookup failed", "err", err, "user", userID)
	}
	return paid
}

// noWatermarkFor snapshots the effective per-download footer preference:
// entitlement alone is not enough, the toggle must also be on.
func (b *Bot) noWatermarkFor(ctx context.Context, userID int64) bool {
	if b.db == nil { // test bots run without a store
		return false
	}
	settings, err := b.db.GetOrCreateUserSettings(ctx, userID)
	if err != nil {
		slog.Warn("user settings lookup failed", "err", err, "user", userID)
		return false
	}
	return settings.NoWatermark && b.wmEntitled(ctx, userID)
}

func (b *Bot) botUsername() string {
	if u := strings.TrimSpace(b.cfg.BotUsername); u != "" {
		return strings.TrimPrefix(u, "@")
	}
	return "saveinator_bot"
}

// onPreCheckoutQuery must answer within 10 seconds or Telegram blocks the
// payment, so it only validates the payload and approves.
func (b *Bot) onPreCheckoutQuery(bot *telego.Bot) func(context.Context, *telego.Bot, telego.PreCheckoutQuery) {
	return func(ctx context.Context, _ *telego.Bot, query telego.PreCheckoutQuery) {
		ok := query.InvoicePayload == watermarkProduct
		params := &telego.AnswerPreCheckoutQueryParams{
			PreCheckoutQueryID: query.ID,
			Ok:                 ok,
		}
		if !ok {
			params.ErrorMessage = locale.Get("watermark.pay_error", "en", nil)
		}
		if err := bot.AnswerPreCheckoutQuery(params); err != nil {
			slog.Warn("answer pre-checkout failed", "err", err)
		}
	}
}

func (b *Bot) onSuccessfulPayment(bot *telego.Bot) func(context.Context, *telego.Bot, telego.Message) {
	return func(ctx context.Context, _ *telego.Bot, msg telego.Message) {
		if msg.From == nil || msg.SuccessfulPayment == nil {
			return
		}
		sp := msg.SuccessfulPayment
		if sp.InvoicePayload != watermarkProduct {
			slog.Warn("unknown invoice payload", "payload", sp.InvoicePayload, "user", msg.From.ID)
			return
		}

		created, err := b.db.RecordPurchase(ctx, msg.From.ID, watermarkProduct, sp.TotalAmount, sp.Currency, sp.TelegramPaymentChargeID)
		if err != nil {
			// The unique charge id keeps this idempotent; a DB failure here is
			// logged but the confirmation is still sent (ledger can be repaired
			// from the charge id in the Telegram payment message).
			slog.Error("purchase record failed", "err", err, "user", msg.From.ID, "charge", sp.TelegramPaymentChargeID)
		}
		if created {
			if err := b.db.SetNoWatermark(ctx, msg.From.ID, true); err != nil {
				slog.Error("enable no_watermark failed", "err", err, "user", msg.From.ID)
			}
			metrics.StarPurchasesTotal.WithLabelValues(watermarkProduct).Inc()
		}

		lang := b.userLang(ctx, msg.From.ID)
		_, _ = bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), locale.Get("watermark.purchased", lang, nil)))
	}
}
