package sender

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/locale"
	"saveinator/internal/metrics"
	"saveinator/internal/tgemoji"
	"saveinator/internal/video"
)

// videoMeta holds probed dimensions/duration and an explicit thumbnail for
// a video file, so Telegram doesn't have to auto-generate its own preview
// (which has produced visibly stretched previews for some renditions even
// though the video itself plays back correctly).
type videoMeta struct {
	width, height, duration int
	thumb                   *inputFileHandle
}

func (m *videoMeta) close() {
	if m != nil && m.thumb != nil {
		m.thumb.close()
	}
}

func probeVideoMeta(path string) *videoMeta {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	m := &videoMeta{}
	if w, h, d, err := video.Probe(ctx, path); err == nil {
		m.width, m.height, m.duration = w, h, int(d)
	}
	if thumbPath, err := video.GenerateThumbnail(ctx, path); err == nil {
		defer os.Remove(thumbPath)
		if tf, err := openInputFile(thumbPath); err == nil {
			m.thumb = tf
		}
	}
	return m
}

type Telegram struct {
	bot         *telego.Bot
	botUsername string
}

func New(bot *telego.Bot) *Telegram {
	return NewWithUsername(bot, "saveinator_bot")
}

func NewWithUsername(bot *telego.Bot, username string) *Telegram {
	if username == "" {
		username = "saveinator_bot"
	}
	return &Telegram{bot: bot, botUsername: username}
}

// ResolveBotUsername returns @handle without @ from Telegram getMe, or fallback.
func ResolveBotUsername(bot *telego.Bot, fallback string) string {
	if fallback == "" {
		fallback = "saveinator_bot"
	}
	me, err := bot.GetMe()
	if err != nil || me.Username == "" {
		return fallback
	}
	return me.Username
}

// EditMessage rewrites a status message. The text is rendered as premium-emoji
// HTML, which also escapes it — status text carries yt-dlp output and media
// titles, so it must never be trusted as markup.
func (t *Telegram) EditMessage(chatID int64, messageID int, text string) error {
	return t.EditMessageMarkup(chatID, messageID, text, nil)
}

func (t *Telegram) EditMessageMarkup(chatID int64, messageID int, text string, markup *telego.InlineKeyboardMarkup) error {
	return metrics.CallTelegram("EditMessageText", func() error {
		_, err := t.bot.EditMessageText(&telego.EditMessageTextParams{
			ChatID:      tu.ID(chatID),
			MessageID:   messageID,
			Text:        tgemoji.Render(text),
			ParseMode:   telego.ModeHTML,
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
	caption := t.buildCaption(title, lang, platform)

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
			meta := probeVideoMeta(path)
			defer meta.close()
			params := &telego.SendVideoParams{
				ChatID:            tu.ID(chatID),
				Video:             file.input,
				Caption:           caption,
				Width:             meta.width,
				Height:            meta.height,
				Duration:          meta.duration,
				SupportsStreaming: true,
			}
			if meta.thumb != nil {
				params.Thumbnail = &meta.thumb.input
			}
			return metrics.CallTelegram("SendVideo", func() error {
				_, err := t.bot.SendVideo(params)
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
	caption := t.buildCaption(title, lang, platform)
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
			meta := probeVideoMeta(path)
			defer meta.close()
			vidParams := tu.Video(chat, file.input).WithCaption(caption).WithReplyMarkup(markup).
				WithWidth(meta.width).WithHeight(meta.height).WithDuration(meta.duration).WithSupportsStreaming()
			if meta.thumb != nil {
				vidParams = vidParams.WithThumbnail(&meta.thumb.input)
			}
			return metrics.CallTelegram("SendVideo", func() error {
				_, err := t.bot.SendVideo(vidParams)
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

func (t *Telegram) SendAudio(chatID int64, path, title, performer string, duration int, thumbnailPath string) error {
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
	if thumbnailPath != "" {
		thumb, thumbErr := openInputFile(thumbnailPath)
		if thumbErr == nil {
			defer thumb.close()
			params.Thumbnail = &thumb.input
		}
	}
	return metrics.CallTelegram("SendAudio", func() error {
		_, err := t.bot.SendAudio(params)
		return err
	})
}

func (t *Telegram) buildCaption(title, lang, platform string) string {
	via := locale.Get("download.via_bot", lang, map[string]string{"bot_username": t.botUsername})
	title = strings.TrimSpace(title)
	if title != "" {
		return title + "\n\n" + via
	}
	return via
}

func buildCaption(title, lang, platform, botUsername string) string {
	return (&Telegram{botUsername: botUsername}).buildCaption(title, lang, platform)
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
