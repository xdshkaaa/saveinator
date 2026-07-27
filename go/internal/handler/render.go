package handler

import (
	"github.com/mymmrac/telego"

	"saveinator/internal/tgemoji"
)

// htmlMessage and editHTMLText are local aliases for the premium-emoji
// renderers, so bot message bodies read the same as the telegoutil helpers
// they replace. See tgemoji for the escaping rules.
func htmlMessage(chatID telego.ChatID, text string) *telego.SendMessageParams {
	return tgemoji.Message(chatID, text)
}

func editHTMLText(bot *telego.Bot, chatID int64, messageID int, text string, markup *telego.InlineKeyboardMarkup) {
	tgemoji.EditText(bot, chatID, messageID, text, markup)
}
