package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/locale"
	"saveinator/internal/metrics"
	"saveinator/internal/tgemoji"
)

func (b *Bot) onSettings(bot *telego.Bot) func(context.Context, *telego.Bot, telego.Message) {
	return func(ctx context.Context, _ *telego.Bot, msg telego.Message) {
		if msg.From == nil {
			return
		}
		metrics.RecordCommand("settings")
		lang := b.userLang(ctx, msg.From.ID)
		text := b.settingsSummary(ctx, msg.From.ID, lang)
		_, _ = bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), text).WithReplyMarkup(b.settingsMenuKeyboard(lang)))
	}
}

func (b *Bot) onSettingsCallback(bot *telego.Bot) func(context.Context, *telego.Bot, telego.CallbackQuery) {
	return func(ctx context.Context, _ *telego.Bot, query telego.CallbackQuery) {
		if query.From.ID == 0 || query.Message == nil {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			return
		}
		lang := b.userLang(ctx, query.From.ID)
		parts := strings.Split(query.Data, "|")
		if len(parts) < 2 || parts[0] != "settings" {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			return
		}

		chat := query.Message.GetChat()
		msgID := query.Message.GetMessageID()
		userID := query.From.ID

		switch parts[1] {
		case "menu":
			b.editSettingsMenu(ctx, bot, userID, chat.ID, msgID, lang)
		case "lang":
			if len(parts) == 3 {
				_ = b.db.SetUserLanguage(ctx, userID, parts[2], "saveinator")
				lang = parts[2]
				b.editSettingsMenu(ctx, bot, userID, chat.ID, msgID, lang)
			} else {
				b.editSettingsLang(bot, chat.ID, msgID, lang)
			}
		case "ratio":
			if len(parts) == 3 {
				_ = b.db.SetYouTubeRatio(ctx, userID, parts[2])
				b.editSettingsMenu(ctx, bot, userID, chat.ID, msgID, lang)
			} else {
				b.editSettingsRatio(bot, chat.ID, msgID, lang)
			}
		case "wm":
			switch {
			case len(parts) == 2:
				b.editSettingsWatermark(ctx, bot, userID, chat.ID, msgID, lang)
			case len(parts) == 3 && (parts[2] == "on" || parts[2] == "off"):
				// Server-side entitlement check: the buttons are only rendered
				// for entitled users, but a forged callback must not toggle.
				if b.wmEntitled(ctx, userID) {
					_ = b.db.SetNoWatermark(ctx, userID, parts[2] == "on")
				}
				b.editSettingsWatermark(ctx, bot, userID, chat.ID, msgID, lang)
			case len(parts) == 3 && parts[2] == "buy":
				b.sendWatermarkInvoice(bot, userID, chat.ID, chat.Type, lang)
			}
		case "reset":
			_ = b.db.ResetUserSettings(ctx, userID)
			lang = "en"
			b.editSettingsMenu(ctx, bot, userID, chat.ID, msgID, lang)
		}

		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
	}
}

func (b *Bot) settingsSummary(ctx context.Context, userID int64, lang string) string {
	settings, _ := b.db.GetOrCreateUserSettings(ctx, userID)
	userLang, _ := b.db.GetUserLanguage(ctx, userID, "saveinator")
	if userLang == "" {
		userLang = lang
	}
	stateKey := "watermark.state_on"
	if settings.NoWatermark {
		stateKey = "watermark.state_off"
	}
	return fmt.Sprintf("%s\n\n%s\n%s\n%s",
		locale.Get("settings.title", lang, nil),
		locale.Get("settings.lang_line", lang, map[string]string{"language": languageLabel(userLang, lang)}),
		locale.Get("settings.ratio_line", lang, map[string]string{"ratio": ratioLabel(settings.YouTubeRatio, lang)}),
		locale.Get("settings.wm_line", lang, map[string]string{"state": locale.Get(stateKey, lang, nil)}),
	)
}

func (b *Bot) editSettingsMenu(ctx context.Context, bot *telego.Bot, userID, chatID int64, messageID int, lang string) {
	_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
		ChatID:      tu.ID(chatID),
		MessageID:   messageID,
		Text:        tgemoji.Render(b.settingsSummary(ctx, userID, lang)),
		ParseMode:   telego.ModeHTML,
		ReplyMarkup: b.settingsMenuKeyboard(lang),
	})
}

func (b *Bot) editSettingsLang(bot *telego.Bot, chatID int64, messageID int, lang string) {
	_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
		ChatID:    tu.ID(chatID),
		MessageID: messageID,
		Text:      tgemoji.Render(locale.Get("settings.lang_prompt", lang, nil)),
		ParseMode: telego.ModeHTML,
		ReplyMarkup: tu.InlineKeyboard(
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton(locale.Get("onboarding.btn_en", lang, nil)).WithCallbackData("settings|lang|en"),
				tu.InlineKeyboardButton(locale.Get("onboarding.btn_ru", lang, nil)).WithCallbackData("settings|lang|ru"),
			),
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton(locale.Get("settings.btn_back", lang, nil)).WithCallbackData("settings|menu"),
			),
		),
	})
}

func (b *Bot) editSettingsRatio(bot *telego.Bot, chatID int64, messageID int, lang string) {
	_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
		ChatID:    tu.ID(chatID),
		MessageID: messageID,
		Text:      tgemoji.Render(locale.Get("settings.ratio_prompt", lang, nil)),
		ParseMode: telego.ModeHTML,
		ReplyMarkup: tu.InlineKeyboard(
			tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("settings.ratio_ask", lang, nil)).WithCallbackData("settings|ratio|ask")),
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton(locale.Get("settings.ratio_16_9", lang, nil)).WithCallbackData("settings|ratio|16_9"),
				tu.InlineKeyboardButton(locale.Get("settings.ratio_21_9", lang, nil)).WithCallbackData("settings|ratio|21_9"),
			),
			tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("settings.ratio_9_16", lang, nil)).WithCallbackData("settings|ratio|9_16")),
			tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("settings.btn_back", lang, nil)).WithCallbackData("settings|menu")),
		),
	})
}

func (b *Bot) settingsMenuKeyboard(lang string) *telego.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("settings.btn_lang", lang, nil)).WithCallbackData("settings|lang")),
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("settings.btn_ratio", lang, nil)).WithCallbackData("settings|ratio")),
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("settings.btn_wm", lang, nil)).WithCallbackData("settings|wm")),
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("settings.btn_reset", lang, nil)).WithCallbackData("settings|reset")),
	)
}

// editSettingsWatermark renders the watermark submenu: a buy button with the
// Stars price for users without the purchase, toggle buttons for entitled
// ones (admins and buyers).
func (b *Bot) editSettingsWatermark(ctx context.Context, bot *telego.Bot, userID, chatID int64, messageID int, lang string) {
	entitled := b.wmEntitled(ctx, userID)
	settings, _ := b.db.GetOrCreateUserSettings(ctx, userID)
	username := b.botUsername()
	price := strconv.Itoa(watermarkPriceStars)

	var text string
	var kb *telego.InlineKeyboardMarkup
	if entitled {
		stateKey := "watermark.state_on"
		if settings.NoWatermark {
			stateKey = "watermark.state_off"
		}
		text = locale.Get("watermark.prompt_unlocked", lang, map[string]string{
			"state":        locale.Get(stateKey, lang, nil),
			"bot_username": username,
		})
		kb = tu.InlineKeyboard(
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton(locale.Get("watermark.btn_enable", lang, nil)).WithCallbackData("settings|wm|on"),
				tu.InlineKeyboardButton(locale.Get("watermark.btn_disable", lang, nil)).WithCallbackData("settings|wm|off"),
			),
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton(locale.Get("settings.btn_back", lang, nil)).WithCallbackData("settings|menu"),
			),
		)
	} else {
		text = locale.Get("watermark.prompt_locked", lang, map[string]string{
			"price":        price,
			"bot_username": username,
		})
		kb = tu.InlineKeyboard(
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton(locale.Get("watermark.btn_buy", lang, map[string]string{"price": price})).WithCallbackData("settings|wm|buy"),
			),
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton(locale.Get("settings.btn_back", lang, nil)).WithCallbackData("settings|menu"),
			),
		)
	}

	_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
		ChatID:      tu.ID(chatID),
		MessageID:   messageID,
		Text:        tgemoji.Render(text),
		ParseMode:   telego.ModeHTML,
		ReplyMarkup: kb,
	})
}

// sendWatermarkInvoice offers the one-time Stars purchase. Invoices can only
// be sent to private chats.
func (b *Bot) sendWatermarkInvoice(bot *telego.Bot, userID, chatID int64, chatType string, lang string) {
	if chatType != telego.ChatTypePrivate {
		_, _ = bot.SendMessage(htmlMessage(tu.ID(chatID), locale.Get("watermark.dm_hint", lang, nil)))
		return
	}
	title := locale.Get("watermark.invoice_title", lang, nil)
	_, err := bot.SendInvoice(&telego.SendInvoiceParams{
		ChatID:        tu.ID(chatID),
		Title:         title,
		Description:   locale.Get("watermark.invoice_description", lang, map[string]string{"bot_username": b.botUsername()}),
		Payload:       watermarkProduct,
		ProviderToken: "",
		Currency:      "XTR",
		Prices:        []telego.LabeledPrice{{Label: title, Amount: watermarkPriceStars}},
	})
	if err != nil {
		slog.Warn("send invoice failed", "err", err, "user", userID)
	}
}

func languageLabel(code, lang string) string {
	if code == "ru" {
		return locale.Get("onboarding.btn_ru", lang, nil)
	}
	return locale.Get("onboarding.btn_en", lang, nil)
}

func ratioLabel(value, lang string) string {
	switch value {
	case "16_9":
		return locale.Get("settings.ratio_16_9", lang, nil)
	case "21_9":
		return locale.Get("settings.ratio_21_9", lang, nil)
	case "9_16":
		return locale.Get("settings.ratio_9_16", lang, nil)
	default:
		return locale.Get("settings.ratio_ask", lang, nil)
	}
}
