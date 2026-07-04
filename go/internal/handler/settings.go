package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/locale"
	"saveinator/internal/metrics"
)

func (b *Bot) onSettings(bot *telego.Bot) func(context.Context, *telego.Bot, telego.Message) {
	return func(ctx context.Context, _ *telego.Bot, msg telego.Message) {
		if msg.From == nil {
			return
		}
		metrics.RecordCommand("settings")
		lang := b.userLang(ctx, msg.From.ID)
		text := b.settingsSummary(ctx, msg.From.ID, lang)
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), text).WithReplyMarkup(b.settingsMenuKeyboard(lang)))
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
		case "quality":
			if len(parts) == 3 {
				_ = b.db.SetYouTubeQuality(ctx, userID, parts[2])
				b.editSettingsMenu(ctx, bot, userID, chat.ID, msgID, lang)
			} else {
				b.editSettingsQuality(bot, chat.ID, msgID, lang)
			}
		case "ratio":
			if len(parts) == 3 {
				_ = b.db.SetYouTubeRatio(ctx, userID, parts[2])
				b.editSettingsMenu(ctx, bot, userID, chat.ID, msgID, lang)
			} else {
				b.editSettingsRatio(bot, chat.ID, msgID, lang)
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
	return fmt.Sprintf("%s\n\n%s\n%s\n%s",
		locale.Get("settings.title", lang, nil),
		locale.Get("settings.lang_line", lang, map[string]string{"language": languageLabel(userLang, lang)}),
		locale.Get("settings.quality_line", lang, map[string]string{"quality": qualityLabel(settings.YouTubeQuality, lang)}),
		locale.Get("settings.ratio_line", lang, map[string]string{"ratio": ratioLabel(settings.YouTubeRatio, lang)}),
	)
}

func (b *Bot) editSettingsMenu(ctx context.Context, bot *telego.Bot, userID, chatID int64, messageID int, lang string) {
	_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
		ChatID:      tu.ID(chatID),
		MessageID:   messageID,
		Text:        b.settingsSummary(ctx, userID, lang),
		ReplyMarkup: b.settingsMenuKeyboard(lang),
	})
}

func (b *Bot) editSettingsLang(bot *telego.Bot, chatID int64, messageID int, lang string) {
	_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
		ChatID:    tu.ID(chatID),
		MessageID: messageID,
		Text:      locale.Get("settings.lang_prompt", lang, nil),
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

func (b *Bot) editSettingsQuality(bot *telego.Bot, chatID int64, messageID int, lang string) {
	_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
		ChatID:    tu.ID(chatID),
		MessageID: messageID,
		Text:      locale.Get("settings.quality_prompt", lang, nil),
		ReplyMarkup: tu.InlineKeyboard(
			tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("settings.quality_ask", lang, nil)).WithCallbackData("settings|quality|ask")),
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton(locale.Get("settings.quality_1080", lang, nil)).WithCallbackData("settings|quality|1080"),
				tu.InlineKeyboardButton(locale.Get("settings.quality_720", lang, nil)).WithCallbackData("settings|quality|720"),
			),
			tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("settings.quality_480", lang, nil)).WithCallbackData("settings|quality|480")),
			tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("settings.btn_back", lang, nil)).WithCallbackData("settings|menu")),
		),
	})
}

func (b *Bot) editSettingsRatio(bot *telego.Bot, chatID int64, messageID int, lang string) {
	_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
		ChatID:    tu.ID(chatID),
		MessageID: messageID,
		Text:      locale.Get("settings.ratio_prompt", lang, nil),
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
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("settings.btn_quality", lang, nil)).WithCallbackData("settings|quality")),
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("settings.btn_ratio", lang, nil)).WithCallbackData("settings|ratio")),
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("settings.btn_reset", lang, nil)).WithCallbackData("settings|reset")),
	)
}

func languageLabel(code, lang string) string {
	if code == "ru" {
		return locale.Get("onboarding.btn_ru", lang, nil)
	}
	return locale.Get("onboarding.btn_en", lang, nil)
}

func qualityLabel(value, lang string) string {
	switch value {
	case "1080":
		return locale.Get("settings.quality_1080", lang, nil)
	case "720":
		return locale.Get("settings.quality_720", lang, nil)
	case "480":
		return locale.Get("settings.quality_480", lang, nil)
	default:
		return locale.Get("settings.quality_ask", lang, nil)
	}
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
