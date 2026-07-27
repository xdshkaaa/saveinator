package worker

import "github.com/mymmrac/telego"

type messageSender interface {
	EditMessage(chatID int64, messageID int, text string) error
	EditMessageMarkup(chatID int64, messageID int, text string, markup *telego.InlineKeyboardMarkup) error
	DeleteMessage(chatID int64, messageID int) error
	SendPhotoAlbum(chatID int64, paths []string, caption string) error
	SendFile(chatID int64, path, title, lang, platform string, animation bool) error
	SendFileWithMarkup(chatID int64, path, title, lang, platform string, animation bool, markup *telego.InlineKeyboardMarkup) error
	SendAudio(chatID int64, path, title, performer string, durationSec int, thumbnailPath string) error
}
