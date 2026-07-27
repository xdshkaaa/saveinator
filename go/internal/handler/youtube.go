package handler

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/linkparser"
	"saveinator/internal/locale"
	"saveinator/internal/metrics"
	"saveinator/internal/queue"
	"saveinator/internal/youtube"
)

// probeTimeout bounds the metadata lookup that precedes the format card. It is
// deliberately far shorter than the download timeout: the user is staring at a
// "fetching info" message the whole time.
const probeTimeout = 30 * time.Second

var defaultQualities = []string{"144", "240", "360", "480", "720", "1080"}

func (b *Bot) handleYouTubeLink(ctx context.Context, bot *telego.Bot, msg telego.Message, lang string, link linkparser.ParsedLink) {
	status, err := bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), locale.Get("youtube.fetching_info", lang, nil)))
	if err != nil {
		return
	}

	meta := b.probeYouTube(ctx, link.URL)
	session := youtube.PendingSession{
		UserID:    msg.From.ID,
		URL:       link.URL,
		ChatID:    msg.Chat.ID,
		MessageID: status.MessageID,
		Lang:      lang,
		Options:   b.youtubeOptions(ctx, meta),
	}
	if meta != nil {
		session.Title = meta.Title
		session.Author = meta.Author()
		session.DurationSec = meta.DurationSec()
	}
	_ = b.ytSessions.Save(ctx, session)

	b.renderFormatCard(ctx, bot, session)
}

// probeYouTube resolves metadata through the shared cache. A failure is not
// fatal: the card degrades to a plain quality list and the download falls back
// to the generic format selector.
func (b *Bot) probeYouTube(ctx context.Context, url string) *youtube.Meta {
	videoID := youtube.VideoID(url)
	if meta := b.ytSessions.GetMeta(ctx, videoID); meta != nil {
		return meta
	}
	meta, err := youtube.Probe(ctx, url, probeTimeout)
	if err != nil {
		slog.Warn("youtube probe failed", "url", url, "err", err)
		return nil
	}
	b.ytSessions.SaveMeta(ctx, meta)
	return meta
}

// youtubeOptions narrows the admin-allowed quality ladder to what the video
// actually offers. Without metadata every allowed quality is offered blind.
func (b *Bot) youtubeOptions(ctx context.Context, meta *youtube.Meta) []youtube.Option {
	allowed := youtube.AllowedHeights(b.runtime.CurrentStringList(ctx, "youtube.allowed_qualities", defaultQualities))
	if opts := youtube.Options(meta, allowed); len(opts) > 0 {
		return opts
	}
	opts := make([]youtube.Option, 0, len(allowed))
	for _, h := range allowed {
		opts = append(opts, youtube.Option{Height: h})
	}
	return opts
}

func (b *Bot) renderFormatCard(ctx context.Context, bot *telego.Bot, session youtube.PendingSession) {
	editHTMLText(bot, session.ChatID, session.MessageID,
		youtube.Card(session.Lang, session.Meta(), session.Options, session.TrimLabel()),
		youtube.FormatKeyboard(
			session.Lang,
			session.Options,
			b.runtime.CurrentBool(ctx, "youtube.mp3_enabled", true),
			b.runtime.CurrentBool(ctx, "youtube.trim_enabled", true),
		),
	)
}

func (b *Bot) onQualityChoice(bot *telego.Bot) func(context.Context, *telego.Bot, telego.CallbackQuery) {
	return func(ctx context.Context, _ *telego.Bot, query telego.CallbackQuery) {
		session, ok := b.youtubeSession(ctx, bot, query)
		if !ok {
			return
		}

		quality, err := strconv.Atoi(strings.TrimPrefix(query.Data, "quality:"))
		if err != nil {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			return
		}
		option, known := session.OptionFor(quality)
		if !known {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).
				WithText(locale.Get("youtube.session_expired", session.Lang, nil)).WithShowAlert())
			return
		}

		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
		b.startYouTubeDownload(ctx, bot, *session, option, false)
	}
}

// onYouTubeAction handles the non-quality buttons on the format card.
func (b *Bot) onYouTubeAction(bot *telego.Bot) func(context.Context, *telego.Bot, telego.CallbackQuery) {
	return func(ctx context.Context, _ *telego.Bot, query telego.CallbackQuery) {
		session, ok := b.youtubeSession(ctx, bot, query)
		if !ok {
			return
		}
		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))

		switch strings.TrimPrefix(query.Data, "yt:") {
		case "mp3":
			b.startYouTubeDownload(ctx, bot, *session, youtube.Option{}, true)
		case "trim":
			updated, err := b.ytSessions.SetAwaitingTrim(ctx, query.From.ID, true)
			if err != nil || updated == nil {
				return
			}
			editHTMLText(bot, updated.ChatID, updated.MessageID,
				locale.Get("youtube.trim_prompt", updated.Lang, nil),
				youtube.TrimPromptKeyboard(updated.Lang))
		case "trimoff":
			updated, err := b.ytSessions.SetTrim(ctx, query.From.ID, 0, 0)
			if err != nil || updated == nil {
				return
			}
			b.renderFormatCard(ctx, bot, *updated)
		}
	}
}

// handleYouTubeTrimInput consumes a typed time range while the user is being
// asked for one. It reports whether the message was consumed; a message that
// carries links is left alone so a new link still starts a new download.
func (b *Bot) handleYouTubeTrimInput(ctx context.Context, bot *telego.Bot, msg telego.Message, lang string) bool {
	session, err := b.ytSessions.Get(ctx, msg.From.ID)
	if err != nil || session == nil || !session.AwaitingTrim {
		return false
	}
	body := strings.TrimSpace(messageBody(msg))
	if body == "" || len(linkparser.ExtractURLs(body)) > 0 {
		_, _ = b.ytSessions.SetAwaitingTrim(ctx, msg.From.ID, false)
		return false
	}

	start, end, err := youtube.ParseRange(body, session.DurationSec)
	if err != nil {
		_, _ = bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), trimErrorText(err, lang)))
		return true
	}

	updated, err := b.ytSessions.SetTrim(ctx, msg.From.ID, start, end)
	if err != nil || updated == nil {
		_, _ = bot.SendMessage(htmlMessage(tu.ID(msg.Chat.ID), locale.Get("youtube.session_expired", lang, nil)))
		return true
	}
	b.renderFormatCard(ctx, bot, *updated)
	return true
}

func trimErrorText(err error, lang string) string {
	switch {
	case errors.Is(err, youtube.ErrTrimOrder):
		return locale.Get("youtube.trim_order", lang, nil)
	case errors.Is(err, youtube.ErrTrimRange):
		return locale.Get("youtube.trim_out_of_range", lang, nil)
	default:
		return locale.Get("youtube.trim_invalid", lang, nil)
	}
}

// youtubeSession loads the live format-card session behind a callback query.
func (b *Bot) youtubeSession(ctx context.Context, bot *telego.Bot, query telego.CallbackQuery) (*youtube.PendingSession, bool) {
	if query.From.ID == 0 || query.Message == nil {
		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
		return nil, false
	}
	session, err := b.ytSessions.Get(ctx, query.From.ID)
	if err != nil || session == nil || session.UserID != query.From.ID {
		lang := b.userLang(ctx, query.From.ID)
		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).
			WithText(locale.Get("youtube.session_expired", lang, nil)).WithShowAlert())
		return nil, false
	}
	if session.Lang == "" {
		session.Lang = b.userLang(ctx, query.From.ID)
	}
	// The card may have been re-sent, so trust the message the user tapped.
	session.MessageID = query.Message.GetMessageID()
	session.ChatID = query.Message.GetChat().ID
	return session, true
}

func (b *Bot) startYouTubeDownload(ctx context.Context, bot *telego.Bot, session youtube.PendingSession, option youtube.Option, audioOnly bool) {
	_ = b.ytSessions.Clear(ctx, session.UserID)

	statusText := youtube.ProcessingAudioMessage(session.Lang)
	if !audioOnly {
		statusText = youtube.ProcessingMessage(session.Lang, option.Height)
	}
	editHTMLText(bot, session.ChatID, session.MessageID, statusText, nil)

	aspectRatio := ""
	if !audioOnly {
		aspectRatio = b.youtubeAspectRatio(ctx, session.UserID)
	}

	if err := b.q.EnqueueDownload(buildYouTubePayload(session, option, audioOnly, aspectRatio)); err != nil {
		slog.Warn("enqueue youtube failed", "err", err)
		_, _ = bot.SendMessage(htmlMessage(tu.ID(session.ChatID), locale.Get("errors.generic", session.Lang, nil)))
		return
	}
	metrics.DownloadsEnqueued.WithLabelValues("youtube").Inc()
}

// buildYouTubePayload turns a tapped card button into a worker job. An
// audio-only job carries no quality or format so it never reaches the video
// pipeline; a video job prefers the exact format id the card advertised and
// falls back to the generic selector when the probe produced none.
func buildYouTubePayload(session youtube.PendingSession, option youtube.Option, audioOnly bool, aspectRatio string) queue.DownloadPayload {
	payload := queue.DownloadPayload{
		URL:       session.URL,
		Platform:  "youtube",
		ChatID:    session.ChatID,
		UserID:    session.UserID,
		MessageID: session.MessageID,
		Lang:      session.Lang,
		Title:     session.Title,
		Author:    session.Author,
		AudioOnly: audioOnly,
		TrimStart: session.TrimStart,
		TrimEnd:   session.TrimEnd,
	}
	if audioOnly {
		return payload
	}
	payload.Quality = option.Height
	payload.AspectRatio = aspectRatio
	payload.FormatID = option.FormatID
	if payload.FormatID == "" {
		payload.FormatID = youtube.BuildFormat(option.Height, aspectRatio)
	}
	return payload
}

// youtubeAspectRatio reads the per-user override. The default ("ask") now means
// "keep the original frame", which skips the ffmpeg re-encode entirely.
func (b *Bot) youtubeAspectRatio(ctx context.Context, userID int64) string {
	settings, err := b.db.GetOrCreateUserSettings(ctx, userID)
	if err != nil {
		slog.Warn("load user settings failed", "err", err)
		return ""
	}
	ratio := settings.YouTubeRatio
	if ratio == "" || ratio == "ask" {
		return ""
	}
	allowed := youtube.RatioSetFromStrings(b.runtime.CurrentStringList(ctx, "youtube.allowed_ratios", []string{"16_9", "21_9", "9_16"}))
	if _, ok := allowed[ratio]; !ok {
		return ""
	}
	return ratio
}
