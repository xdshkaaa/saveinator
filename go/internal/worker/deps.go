package worker

import "github.com/mymmrac/telego"

type messageSender interface {
	EditMessage(chatID int64, messageID int, text string) error
	// EditMessageHTML edits a message whose text is trusted, pre-escaped
	// HTML (article links); EditMessageMarkup escapes plain-text statuses.
	EditMessageHTML(chatID int64, messageID int, text string, markup *telego.InlineKeyboardMarkup) error
	EditMessageMarkup(chatID int64, messageID int, text string, markup *telego.InlineKeyboardMarkup) error
	DeleteMessage(chatID int64, messageID int) error
	SendMessageMarkup(chatID int64, text string, markup *telego.InlineKeyboardMarkup) (*telego.Message, error)
	SendPhotoAlbum(chatID int64, paths []string, caption string) error
	SendFile(chatID int64, path, title, lang, platform string, animation bool) error
	SendFileNoFooter(chatID int64, path, title, lang, platform string, animation bool) error
	SendFileWithMarkup(chatID int64, path, title, lang, platform string, animation bool, markup *telego.InlineKeyboardMarkup) error
	SendAudio(chatID int64, path, title, performer string, durationSec int, thumbnailPath string) error
}
