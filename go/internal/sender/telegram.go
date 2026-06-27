package sender

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/locale"
	"saveinator/internal/metrics"
)

type Telegram struct {
	bot *telego.Bot
}

func New(bot *telego.Bot) *Telegram {
	return &Telegram{bot: bot}
}

func (t *Telegram) EditMessage(chatID int64, messageID int, text string) error {
	return metrics.CallTelegram("EditMessageText", func() error {
		_, err := t.bot.EditMessageText(&telego.EditMessageTextParams{
			ChatID:    tu.ID(chatID),
			MessageID: messageID,
			Text:      text,
		})
		return err
	})
}

func (t *Telegram) EditMessageMarkup(chatID int64, messageID int, text string, markup *telego.InlineKeyboardMarkup) error {
	if markup == nil {
		return t.EditMessage(chatID, messageID, text)
	}
	return metrics.CallTelegram("EditMessageText", func() error {
		_, err := t.bot.EditMessageText(&telego.EditMessageTextParams{
			ChatID:      tu.ID(chatID),
			MessageID:   messageID,
			Text:        text,
			ReplyMarkup: markup,
		})
		return err
	})
}

func (t *Telegram) DeleteMessage(chatID int64, messageID int) error {
	return metrics.CallTelegram("DeleteMessage", func() error {
		return t.bot.DeleteMessage(&telego.DeleteMessageParams{
			ChatID:    tu.ID(chatID),
			MessageID: messageID,
		})
	})
}

func (t *Telegram) SendFile(chatID int64, path, title, lang, platform string, animation bool) error {
	file, err := openInputFile(path)
	if err != nil {
		return err
	}
	defer file.close()

	sizeMB := float64(fileSize(path)) / (1024 * 1024)
	caption := buildCaption(title, lang, platform)

	if animation {
		return metrics.CallTelegram("SendAnimation", func() error {
			_, err := t.bot.SendAnimation(&telego.SendAnimationParams{
				ChatID:    tu.ID(chatID),
				Animation: file.input,
				Caption:   caption,
			})
			return err
		})
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		return metrics.CallTelegram("SendPhoto", func() error {
			_, err := t.bot.SendPhoto(&telego.SendPhotoParams{
				ChatID:  tu.ID(chatID),
				Photo:   file.input,
				Caption: caption,
			})
			return err
		})
	case ".mp4", ".webm", ".mov", ".mkv", ".m4v":
		if sizeMB <= 50 {
			return metrics.CallTelegram("SendVideo", func() error {
				_, err := t.bot.SendVideo(&telego.SendVideoParams{
					ChatID:  tu.ID(chatID),
					Video:   file.input,
					Caption: caption,
				})
				return err
			})
		}
	}

	return metrics.CallTelegram("SendDocument", func() error {
		_, err := t.bot.SendDocument(&telego.SendDocumentParams{
			ChatID:   tu.ID(chatID),
			Document: file.input,
			Caption:  caption,
		})
		return err
	})
}

func (t *Telegram) SendFileWithMarkup(chatID int64, path, title, lang, platform string, animation bool, markup *telego.InlineKeyboardMarkup) error {
	if markup == nil {
		return t.SendFile(chatID, path, title, lang, platform, animation)
	}

	file, err := openInputFile(path)
	if err != nil {
		return err
	}
	defer file.close()

	sizeMB := float64(fileSize(path)) / (1024 * 1024)
	caption := buildCaption(title, lang, platform)
	chat := tu.ID(chatID)

	if animation {
		return metrics.CallTelegram("SendAnimation", func() error {
			_, err := t.bot.SendAnimation(tu.Animation(chat, file.input).WithCaption(caption).WithReplyMarkup(markup))
			return err
		})
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		return metrics.CallTelegram("SendPhoto", func() error {
			_, err := t.bot.SendPhoto(tu.Photo(chat, file.input).WithCaption(caption).WithReplyMarkup(markup))
			return err
		})
	case ".mp4", ".webm", ".mov", ".mkv", ".m4v":
		if sizeMB <= 50 {
			return metrics.CallTelegram("SendVideo", func() error {
				_, err := t.bot.SendVideo(tu.Video(chat, file.input).WithCaption(caption).WithReplyMarkup(markup))
				return err
			})
		}
	}

	return metrics.CallTelegram("SendDocument", func() error {
		_, err := t.bot.SendDocument(tu.Document(chat, file.input).WithCaption(caption).WithReplyMarkup(markup))
		return err
	})
}

func (t *Telegram) SendPhotoAlbum(chatID int64, paths []string, caption string) error {
	if len(paths) == 0 {
		return nil
	}
	if len(paths) == 1 {
		return t.SendFile(chatID, paths[0], "", "", "", false)
	}

	const chunk = 10
	for i := 0; i < len(paths); i += chunk {
		end := i + chunk
		if end > len(paths) {
			end = len(paths)
		}
		media := make([]telego.InputMedia, 0, end-i)
		for j, p := range paths[i:end] {
			file, err := openInputFile(p)
			if err != nil {
				return err
			}
			photo := tu.MediaPhoto(file.input)
			if i == 0 && j == 0 && caption != "" {
				photo = photo.WithCaption(caption)
			}
			media = append(media, photo)
			file.close()
		}
		err := metrics.CallTelegram("SendMediaGroup", func() error {
			_, err := t.bot.SendMediaGroup(&telego.SendMediaGroupParams{
				ChatID: tu.ID(chatID),
				Media:  media,
			})
			return err
		})
		if err != nil {
			for _, p := range paths[i:end] {
				if sendErr := t.SendFile(chatID, p, "", "", "", false); sendErr != nil {
					return sendErr
				}
			}
		}
	}
	return nil
}

type inputFileHandle struct {
	input telego.InputFile
	file  *os.File
}

func (h *inputFileHandle) close() {
	if h.file != nil {
		_ = h.file.Close()
	}
}

func openInputFile(path string) (*inputFileHandle, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	return &inputFileHandle{
		input: tu.File(tu.NameReader(f, filepath.Base(path))),
		file:  f,
	}, nil
}

func (t *Telegram) SendAudio(chatID int64, path, title, performer string, duration int) error {
	file, err := openInputFile(path)
	if err != nil {
		return err
	}
	defer file.close()

	params := &telego.SendAudioParams{
		ChatID: tu.ID(chatID),
		Audio:  file.input,
		Title:  title,
	}
	if performer != "" {
		params.Performer = performer
	}
	if duration > 0 {
		params.Duration = duration
	}
	return metrics.CallTelegram("SendAudio", func() error {
		_, err := t.bot.SendAudio(params)
		return err
	})
}

func buildCaption(title, lang, platform string) string {
	via := locale.Get("download.via_bot", lang, map[string]string{"bot_username": "saveinator_bot"})
	title = strings.TrimSpace(title)
	if title != "" {
		return title + "\n\n" + via
	}
	if platform == "tiktok" {
		return via
	}
	return ""
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
