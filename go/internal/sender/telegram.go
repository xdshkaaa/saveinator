package sender

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoapi"
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

// EditMessageHTML rewrites a message whose text is already valid HTML
// assembled by the caller (links, escaped titles). Unlike EditMessageMarkup
// it does not escape the text — tgemoji.Decorate only upgrades covered emoji.
func (t *Telegram) EditMessageHTML(chatID int64, messageID int, text string, markup *telego.InlineKeyboardMarkup) error {
	return metrics.CallTelegram("EditMessageText", func() error {
		_, err := t.bot.EditMessageText(&telego.EditMessageTextParams{
			ChatID:      tu.ID(chatID),
			MessageID:   messageID,
			Text:        tgemoji.Decorate(text),
			ParseMode:   telego.ModeHTML,
			ReplyMarkup: markup,
		})
		return err
	})
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

// SendMessageMarkup delivers a text message with an optional inline keyboard.
// The text must be valid HTML (interpolated values pre-escaped by the caller);
// it is passed through tgemoji.Decorate, which only upgrades covered emoji.
func (t *Telegram) SendMessageMarkup(chatID int64, text string, markup *telego.InlineKeyboardMarkup) (*telego.Message, error) {
	params := tu.Message(tu.ID(chatID), tgemoji.Decorate(text)).WithParseMode(telego.ModeHTML)
	if markup != nil {
		params = params.WithReplyMarkup(markup)
	}
	var msg *telego.Message
	err := metrics.CallTelegram("SendMessage", func() error {
		m, err := t.bot.SendMessage(params)
		msg = m
		return err
	})
	return msg, err
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
	return t.sendFile(chatID, path, t.buildCaption(title, lang, platform), animation, nil)
}

// SendFileNoFooter delivers media with the title-only caption: no "via @bot"
// line. Used for users who turned the watermark (bot signature) off.
func (t *Telegram) SendFileNoFooter(chatID int64, path, title, lang, platform string, animation bool) error {
	return t.sendFile(chatID, path, buildCleanCaption(title), animation, nil)
}

func (t *Telegram) SendFileWithMarkup(chatID int64, path, title, lang, platform string, animation bool, markup *telego.InlineKeyboardMarkup) error {
	if markup == nil {
		return t.SendFile(chatID, path, title, lang, platform, animation)
	}
	return t.sendFile(chatID, path, t.buildCaption(title, lang, platform), animation, markup)
}

func (t *Telegram) sendFile(chatID int64, path, caption string, animation bool, markup *telego.InlineKeyboardMarkup) error {
	file, err := openInputFile(path)
	if err != nil {
		return err
	}
	defer file.close()

	// ReplyMarkup params are interface-typed in telego: chaining WithReplyMarkup
	// with a typed-nil *InlineKeyboardMarkup wraps it into a non-nil interface,
	// and the request goes out with "reply_markup": null which Telegram rejects
	// ("object expected as reply markup"). Only attach real keyboards.
	sizeMB := float64(fileSize(path)) / (1024 * 1024)
	chat := tu.ID(chatID)

	if animation {
		anim := tu.Animation(chat, file.input).WithCaption(caption)
		if markup != nil {
			anim = anim.WithReplyMarkup(markup)
		}
		return resendWithoutCaption(
			func() {
				file.rewind()
				anim.Caption = ""
			},
			func() error {
				return metrics.CallTelegram("SendAnimation", func() error {
					_, err := t.bot.SendAnimation(anim)
					return err
				})
			},
		)
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		photo := tu.Photo(chat, file.input).WithCaption(caption)
		if markup != nil {
			photo = photo.WithReplyMarkup(markup)
		}
		return resendWithoutCaption(
			func() {
				file.rewind()
				photo.Caption = ""
			},
			func() error {
				return metrics.CallTelegram("SendPhoto", func() error {
					_, err := t.bot.SendPhoto(photo)
					return err
				})
			},
		)
	case ".mp4", ".webm", ".mov", ".mkv", ".m4v":
		if sizeMB <= 50 {
			meta := probeVideoMeta(path)
			defer meta.close()
			vidParams := tu.Video(chat, file.input).WithCaption(caption).
				WithWidth(meta.width).WithHeight(meta.height).WithDuration(meta.duration).WithSupportsStreaming()
			if markup != nil {
				vidParams = vidParams.WithReplyMarkup(markup)
			}
			if meta.thumb != nil {
				vidParams = vidParams.WithThumbnail(&meta.thumb.input)
			}
			return resendWithoutCaption(
				func() {
					file.rewind()
					if meta.thumb != nil {
						meta.thumb.rewind()
					}
					vidParams.Caption = ""
				},
				func() error {
					return metrics.CallTelegram("SendVideo", func() error {
						_, err := t.bot.SendVideo(vidParams)
						return err
					})
				},
			)
		}
	}

	doc := tu.Document(chat, file.input).WithCaption(caption)
	if markup != nil {
		doc = doc.WithReplyMarkup(markup)
	}
	return resendWithoutCaption(
		func() {
			file.rewind()
			doc.Caption = ""
		},
		func() error {
			return metrics.CallTelegram("SendDocument", func() error {
				_, err := t.bot.SendDocument(doc)
				return err
			})
		},
	)
}

// isCaptionTooLong reports whether Telegram refused the media because its
// caption exceeded the caption limit (400 "message caption is too long").
func isCaptionTooLong(err error) bool {
	var apiErr *telegoapi.Error
	return errors.As(err, &apiErr) &&
		apiErr.ErrorCode == 400 &&
		strings.Contains(apiErr.Description, "caption is too long")
}

// resendWithoutCaption runs send once and, if Telegram rejected the caption as
// too long, prepares a retry (rewinding every uploaded file handle and clearing
// the caption) and repeats the request, delivering the media without any
// caption.
func resendWithoutCaption(prepareRetry func(), send func() error) error {
	err := send()
	if !isCaptionTooLong(err) {
		return err
	}
	slog.Warn("telegram: caption too long, resending media without caption", "err", err)
	prepareRetry()
	return send()
}

// sendPhoto delivers a single image with a ready-made caption. Used by
// SendPhotoAlbum so a one-photo post keeps its exact caption (the caption is
// already fully assembled by the caller and must not be wrapped again).
func (t *Telegram) sendPhoto(chatID int64, path, caption string) error {
	file, err := openInputFile(path)
	if err != nil {
		return err
	}
	defer file.close()

	params := &telego.SendPhotoParams{
		ChatID:  tu.ID(chatID),
		Photo:   file.input,
		Caption: caption,
	}
	return resendWithoutCaption(
		func() {
			file.rewind()
			params.Caption = ""
		},
		func() error {
			return metrics.CallTelegram("SendPhoto", func() error {
				_, err := t.bot.SendPhoto(params)
				return err
			})
		},
	)
}

func (t *Telegram) SendPhotoAlbum(chatID int64, paths []string, caption string) error {
	if len(paths) == 0 {
		return nil
	}
	if len(paths) == 1 {
		return t.sendPhoto(chatID, paths[0], caption)
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
			for j, p := range paths[i:end] {
				var sendErr error
				if i == 0 && j == 0 && caption != "" {
					sendErr = t.sendPhoto(chatID, p, caption)
				} else {
					sendErr = t.sendPhoto(chatID, p, "")
				}
				if sendErr != nil {
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

// rewind seeks the underlying file back to the start: telego copies the reader
// into a new multipart form on every request, so a retried upload without this
// would ship an empty body.
func (h *inputFileHandle) rewind() {
	if h.file != nil {
		_, _ = h.file.Seek(0, io.SeekStart)
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

// buildCleanCaption returns the title-only caption, without the "via @bot"
// footer, for users who bought the watermark removal.
func buildCleanCaption(title string) string {
	return strings.TrimSpace(title)
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
