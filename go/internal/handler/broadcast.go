package handler

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strconv"
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/locale"
	"saveinator/internal/metrics"
	"saveinator/internal/queue"
)

func (b *Bot) onBroadcast(bot *telego.Bot) func(context.Context, *telego.Bot, telego.Message) {
	return func(ctx context.Context, _ *telego.Bot, msg telego.Message) {
		if msg.From == nil || !b.isAdmin(msg.From.ID) {
			return
		}
		metrics.RecordCommand("broadcast")
		b.fsm.Clear(msg.From.ID)
		lang := b.userLang(ctx, msg.From.ID)
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("broadcast.menu_title", lang, nil)).
			WithParseMode(telego.ModeHTML).
			WithReplyMarkup(b.broadcastMenuKeyboard(lang)))
	}
}

func (b *Bot) onBroadcastCallback(bot *telego.Bot) func(context.Context, *telego.Bot, telego.CallbackQuery) {
	return func(ctx context.Context, _ *telego.Bot, query telego.CallbackQuery) {
		if query.From.ID == 0 || !b.isAdmin(query.From.ID) {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			return
		}
		if query.Message == nil {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			return
		}

		lang := b.userLang(ctx, query.From.ID)
		parts := strings.Split(query.Data, "|")
		if len(parts) < 2 || parts[0] != "broadcast" {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			return
		}

		chatID := query.Message.GetChat().ID
		msgID := query.Message.GetMessageID()

		switch parts[1] {
		case "menu":
			b.fsm.Clear(query.From.ID)
			b.editAdminText(bot, chatID, msgID, locale.Get("broadcast.menu_title", lang, nil), b.broadcastMenuKeyboard(lang))
		case "create":
			b.fsm.Set(query.From.ID, "broadcast_text", nil)
			b.editAdminText(bot, chatID, msgID, locale.Get("broadcast.enter_text", lang, nil), tu.InlineKeyboard(
				tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("broadcast.btn_back", lang, nil)).WithCallbackData("broadcast|menu")),
			))
		case "history":
			b.showBroadcastHistory(ctx, bot, chatID, msgID, lang)
		case "status":
			b.showBroadcastStatus(ctx, bot, chatID, msgID, lang)
		case "audience":
			if len(parts) < 4 {
				break
			}
			bid, _ := strconv.Atoi(parts[2])
			audience := parts[3]
			if audience == "test" {
				bc, err := b.db.GetBroadcast(ctx, bid)
				if err != nil {
					_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText("Broadcast not found."))
					return
				}
				_, err = bot.SendMessage(tu.Message(tu.ID(query.From.ID), bc.Text))
				if err != nil {
					_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText("Failed to send test message."))
					return
				}
				_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText(locale.Get("broadcast.test_sent", lang, nil)))
				return
			}
			b.showBroadcastPreview(ctx, bot, query, bid, audience, lang)
			return
		case "send":
			if len(parts) < 4 {
				break
			}
			bid, _ := strconv.Atoi(parts[2])
			audience := parts[3]
			b.queueBroadcast(ctx, bot, query, bid, audience, lang)
			return
		case "edit_text":
			if len(parts) < 3 {
				break
			}
			bid := parts[2]
			b.fsm.Set(query.From.ID, "broadcast_edit", map[string]string{"broadcast_id": bid})
			b.editAdminText(bot, chatID, msgID, locale.Get("broadcast.enter_text", lang, nil), tu.InlineKeyboard(
				tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("broadcast.btn_back", lang, nil)).WithCallbackData("broadcast|menu")),
			))
		case "cancel":
			if len(parts) < 3 {
				break
			}
			bid, _ := strconv.Atoi(parts[2])
			_ = b.db.UpdateBroadcastStatus(ctx, bid, "CANCELLED", 0)
			b.fsm.Clear(query.From.ID)
			b.editAdminText(bot, chatID, msgID, locale.Get("broadcast.cancelled", lang, nil), b.broadcastMenuKeyboard(lang))
		}
		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
	}
}

func (b *Bot) broadcastMenuKeyboard(lang string) *telego.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("broadcast.btn_create", lang, nil)).WithCallbackData("broadcast|create")),
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("broadcast.btn_history", lang, nil)).WithCallbackData("broadcast|history")),
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("broadcast.btn_status", lang, nil)).WithCallbackData("broadcast|status")),
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("broadcast.btn_back", lang, nil)).WithCallbackData("admin|menu")),
	)
}

func (b *Bot) broadcastSaveText(ctx context.Context, bot *telego.Bot, msg telego.Message, lang, text string, existingID int) bool {
	if strings.TrimSpace(text) == "" {
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), "Text cannot be empty."))
		return true
	}
	b.fsm.Clear(msg.From.ID)
	var id int
	var err error
	if existingID > 0 {
		id = existingID
		err = b.db.UpdateBroadcastText(ctx, id, text)
	} else {
		id, err = b.db.CreateBroadcast(ctx, msg.From.ID, text)
	}
	if err != nil {
		slog.Warn("broadcast create failed", "err", err)
		return true
	}
	b.showAudienceSelection(bot, msg.Chat.ID, id, lang)
	return true
}

func (b *Bot) showAudienceSelection(bot *telego.Bot, chatID int64, broadcastID int, lang string) {
	text := locale.Get("broadcast.audience_prompt", lang, nil)
	kb := tu.InlineKeyboard(
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("broadcast.audience_all", lang, nil)).WithCallbackData(fmt.Sprintf("broadcast|audience|%d|all", broadcastID))),
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("broadcast.audience_ru", lang, nil)).WithCallbackData(fmt.Sprintf("broadcast|audience|%d|ru", broadcastID))),
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("broadcast.audience_en", lang, nil)).WithCallbackData(fmt.Sprintf("broadcast|audience|%d|en", broadcastID))),
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("broadcast.audience_active", lang, nil)).WithCallbackData(fmt.Sprintf("broadcast|audience|%d|active", broadcastID))),
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("broadcast.audience_test", lang, nil)).WithCallbackData(fmt.Sprintf("broadcast|audience|%d|test", broadcastID))),
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("broadcast.btn_back", lang, nil)).WithCallbackData("broadcast|menu")),
	)
	_, _ = bot.SendMessage(tu.Message(tu.ID(chatID), text).WithReplyMarkup(kb))
}

func (b *Bot) showBroadcastPreview(ctx context.Context, bot *telego.Bot, query telego.CallbackQuery, broadcastID int, audience, lang string) {
	bc, err := b.db.GetBroadcast(ctx, broadcastID)
	if err != nil {
		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText("Broadcast not found."))
		return
	}
	total, _ := b.db.CountBroadcastRecipients(ctx, audience)
	preview := locale.Get("broadcast.preview_title", lang, map[string]string{"text": html.EscapeString(bc.Text)}) + "\n\n" +
		locale.Get("broadcast.preview_audience", lang, map[string]string{"audience": broadcastAudienceLabel(audience, lang)}) + "\n" +
		locale.Get("broadcast.preview_recipients", lang, map[string]string{"count": strconv.Itoa(total)}) + "\n\n" +
		locale.Get("broadcast.preview_confirm", lang, nil)

	kb := tu.InlineKeyboard(
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("broadcast.btn_send", lang, nil)).WithCallbackData(fmt.Sprintf("broadcast|send|%d|%s", broadcastID, audience))),
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("broadcast.btn_edit_text", lang, nil)).WithCallbackData(fmt.Sprintf("broadcast|edit_text|%d", broadcastID))),
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("broadcast.btn_test", lang, nil)).WithCallbackData(fmt.Sprintf("broadcast|audience|%d|test", broadcastID))),
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("broadcast.btn_cancel", lang, nil)).WithCallbackData(fmt.Sprintf("broadcast|cancel|%d", broadcastID))),
	)
	b.editAdminText(bot, query.Message.GetChat().ID, query.Message.GetMessageID(), preview, kb)
	_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
}

func (b *Bot) queueBroadcast(ctx context.Context, bot *telego.Bot, query telego.CallbackQuery, broadcastID int, audience, lang string) {
	ids, err := b.db.BroadcastRecipientIDs(ctx, audience)
	if err != nil {
		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText("Failed to load recipients."))
		return
	}
	_ = b.db.UpdateBroadcastStatus(ctx, broadcastID, "QUEUED", len(ids))
	if err := b.q.EnqueueBroadcast(queue.BroadcastPayload{
		BroadcastID: broadcastID,
		Audience:    audience,
		UserIDs:     ids,
	}); err != nil {
		slog.Warn("enqueue broadcast failed", "err", err)
		_ = b.db.FailBroadcast(ctx, broadcastID)
	} else {
		metrics.RecordBroadcastCreated(audience)
	}
	text := locale.Get("broadcast.starting", lang, map[string]string{
		"audience":   broadcastAudienceLabel(audience, lang),
		"recipients": strconv.Itoa(len(ids)),
	})
	b.editAdminText(bot, query.Message.GetChat().ID, query.Message.GetMessageID(), text, b.broadcastMenuKeyboard(lang))
	_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
}

func (b *Bot) showBroadcastHistory(ctx context.Context, bot *telego.Bot, chatID int64, msgID int, lang string) {
	list, err := b.db.ListBroadcasts(ctx, 20)
	if err != nil || len(list) == 0 {
		b.editAdminText(bot, chatID, msgID, locale.Get("broadcast.history_empty", lang, nil), tu.InlineKeyboard(
			tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("broadcast.btn_back", lang, nil)).WithCallbackData("broadcast|menu")),
		))
		return
	}
	lines := []string{locale.Get("broadcast.history_title", lang, nil)}
	for _, bc := range list {
		created := bc.CreatedAt.Format("2006-01-02 15:04")
		lines = append(lines, locale.Get("broadcast.history_line", lang, map[string]string{
			"id":      strconv.Itoa(bc.ID),
			"status":  broadcastStatusLabel(bc.Status, lang),
			"audience": broadcastAudienceLabel(strings.ToLower(bc.Audience), lang),
			"sent":    strconv.Itoa(bc.SentCount),
			"total":   strconv.Itoa(bc.TotalRecipients),
			"created": created,
		}))
	}
	b.editAdminText(bot, chatID, msgID, strings.Join(lines, "\n"), tu.InlineKeyboard(
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("broadcast.btn_back", lang, nil)).WithCallbackData("broadcast|menu")),
	))
}

func (b *Bot) showBroadcastStatus(ctx context.Context, bot *telego.Bot, chatID int64, msgID int, lang string) {
	active, err := b.db.GetActiveBroadcast(ctx)
	if err != nil || active == nil {
		b.editAdminText(bot, chatID, msgID, locale.Get("broadcast.no_active", lang, nil), tu.InlineKeyboard(
			tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("broadcast.btn_back", lang, nil)).WithCallbackData("broadcast|menu")),
		))
		return
	}
	text := locale.Get("broadcast.active_title", lang, map[string]string{
		"id":       strconv.Itoa(active.ID),
		"status":   broadcastStatusLabel(active.Status, lang),
		"audience": broadcastAudienceLabel(strings.ToLower(active.Audience), lang),
		"sent":     strconv.Itoa(active.SentCount),
		"total":    strconv.Itoa(active.TotalRecipients),
		"failed":   strconv.Itoa(active.FailedCount),
		"blocked":  strconv.Itoa(active.BlockedCount),
	})
	b.editAdminText(bot, chatID, msgID, text, tu.InlineKeyboard(
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("broadcast.btn_back", lang, nil)).WithCallbackData("broadcast|menu")),
	))
}

func broadcastAudienceLabel(audience, lang string) string {
	key := "broadcast.audience_label_" + audience
	label := locale.Get(key, lang, nil)
	if label == key {
		return audience
	}
	return label
}

func broadcastStatusLabel(status, lang string) string {
	key := "broadcast.status_" + strings.ToLower(status)
	label := locale.Get(key, lang, nil)
	if label == key {
		return status
	}
	return label
}
