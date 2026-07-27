package tgemoji

import (
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// Message mirrors telegoutil.Message but renders the body with the premium
// emoji pack. Rendering escapes the text first, so interpolated values (media
// titles, usernames, yt-dlp output, admin input) cannot break the markup.
//
// Only message bodies go through here. Inline keyboard labels and
// answerCallbackQuery texts carry no entities and must stay plain.
func Message(chatID telego.ChatID, text string) *telego.SendMessageParams {
	return tu.Message(chatID, Render(text)).WithParseMode(telego.ModeHTML)
}

// EditText rewrites a message body with the same rendering rules.
func EditText(bot *telego.Bot, chatID int64, messageID int, text string, markup *telego.InlineKeyboardMarkup) {
	_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
		ChatID:      tu.ID(chatID),
		MessageID:   messageID,
		Text:        Render(text),
		ParseMode:   telego.ModeHTML,
		ReplyMarkup: markup,
	})
}
