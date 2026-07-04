package botkit

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
			if len(parts) == 3 && b.bc.langAllowed(parts[2]) {
				_ = b.db.SetUserLanguage(ctx, userID, parts[2], b.bc.Slug)
				lang = parts[2]
				b.editSettingsMenu(ctx, bot, userID, chat.ID, msgID, lang)
			} else {
				b.editSettingsLang(bot, chat.ID, msgID, lang)
			}
		}

		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
	}
}

func (b *Bot) settingsSummary(ctx context.Context, userID int64, lang string) string {
	userLang, _ := b.db.GetUserLanguage(ctx, userID, b.bc.Slug)
	if userLang == "" {
		userLang = lang
	}
	return fmt.Sprintf("%s\n\n%s",
		locale.Get("settings.title", lang, nil),
		locale.Get("settings.lang_line", lang, map[string]string{"language": languageLabel(userLang, lang)}),
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
	var buttons []telego.InlineKeyboardButton
	for _, code := range b.bc.Languages {
		buttons = append(buttons, tu.InlineKeyboardButton(locale.SelfName(code)).
			WithCallbackData("settings|lang|"+code))
	}
	_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
		ChatID:    tu.ID(chatID),
		MessageID: messageID,
		Text:      locale.Get("settings.lang_prompt", lang, nil),
		ReplyMarkup: tu.InlineKeyboard(
			tu.InlineKeyboardRow(buttons...),
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton(locale.Get("settings.btn_back", lang, nil)).WithCallbackData("settings|menu"),
			),
		),
	})
}

func (b *Bot) settingsMenuKeyboard(lang string) *telego.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("settings.btn_lang", lang, nil)).WithCallbackData("settings|lang")),
	)
}

func languageLabel(code, _ string) string {
	return locale.SelfName(code)
}
