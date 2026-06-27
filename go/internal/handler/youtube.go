package handler

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/linkparser"
	"saveinator/internal/locale"
	"saveinator/internal/metrics"
	"saveinator/internal/queue"
	"saveinator/internal/youtube"
)

func (b *Bot) handleYouTubeLink(ctx context.Context, bot *telego.Bot, msg telego.Message, lang string, link linkparser.ParsedLink) {
	settings, err := b.db.GetOrCreateUserSettings(ctx, msg.From.ID)
	if err != nil {
		slog.Warn("load user settings failed", "err", err)
	}

	allowedQualities := b.runtime.CurrentStringList(ctx, "youtube.allowed_qualities", []string{"1080", "720", "480"})
	allowedRatios := b.runtime.CurrentStringList(ctx, "youtube.allowed_ratios", []string{"16_9", "21_9", "9_16"})
	validQualities := youtube.QualitySetFromStrings(allowedQualities)
	validRatios := youtube.RatioSetFromStrings(allowedRatios)

	autoQuality := parseAutoQuality(settings.YouTubeQuality, validQualities)
	if autoQuality == nil {
		defaultQuality := b.runtime.CurrentString(ctx, "youtube.default_quality", "ask")
		if defaultQuality != "ask" && defaultQuality != "" {
			autoQuality = parseAutoQuality(defaultQuality, validQualities)
		}
	}
	autoRatio := settings.YouTubeRatio
	if autoRatio == "ask" {
		autoRatio = ""
	}
	if autoRatio != "" {
		if _, ok := validRatios[autoRatio]; !ok {
			autoRatio = ""
		}
	}
	if linkparser.IsYouTubeShorts(link.URL) {
		if _, ok := validRatios["9_16"]; ok {
			autoRatio = "9_16"
		}
	}

	if autoQuality != nil && autoRatio != "" {
		_, _ = bot.SendMessage(tu.Message(
			tu.ID(msg.Chat.ID),
			youtube.ProcessingMessage(lang, *autoQuality, autoRatio),
		))
		status, err := bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("download.downloading", lang, nil)))
		if err != nil {
			return
		}
		b.enqueueYouTube(ctx, bot, msg.From.ID, msg.Chat.ID, status.MessageID, lang, link.URL, *autoQuality, autoRatio)
		return
	}

	if autoQuality != nil {
		session := youtube.PendingSession{
			UserID:    msg.From.ID,
			URL:       link.URL,
			ChatID:    msg.Chat.ID,
			MessageID: 0,
			Lang:      lang,
			Quality:   autoQuality,
		}
		_ = b.ytSessions.Save(ctx, session)
		status, err := bot.SendMessage(tu.Message(
			tu.ID(msg.Chat.ID),
			locale.Get("youtube.choose_ratio", lang, nil),
		).WithReplyMarkup(youtube.RatioKeyboardFrom(lang, allowedRatios)))
		if err == nil {
			session.MessageID = status.MessageID
			_ = b.ytSessions.Save(ctx, session)
		}
		return
	}

	status, err := bot.SendMessage(tu.Message(
		tu.ID(msg.Chat.ID),
		locale.Get("youtube.choose_quality", lang, nil),
	).WithReplyMarkup(youtube.QualityKeyboardFrom(lang, allowedQualities)))
	if err != nil {
		return
	}
	_ = b.ytSessions.Save(ctx, youtube.PendingSession{
		UserID:    msg.From.ID,
		URL:       link.URL,
		ChatID:    msg.Chat.ID,
		MessageID: status.MessageID,
		Lang:      lang,
	})
}

func (b *Bot) onQualityChoice(bot *telego.Bot) func(context.Context, *telego.Bot, telego.CallbackQuery) {
	return func(ctx context.Context, _ *telego.Bot, query telego.CallbackQuery) {
		if query.From.ID == 0 || query.Message == nil {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			return
		}
		lang := b.userLang(ctx, query.From.ID)
		allowedQualities := youtube.QualitySetFromStrings(b.runtime.CurrentStringList(ctx, "youtube.allowed_qualities", []string{"1080", "720", "480"}))
		session, err := b.ytSessions.Get(ctx, query.From.ID)
		if err != nil || session == nil || session.UserID != query.From.ID {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText(locale.Get("youtube.session_expired", lang, nil)).WithShowAlert())
			return
		}

		qualityStr := strings.TrimPrefix(query.Data, "quality:")
		quality, err := strconv.Atoi(qualityStr)
		if err != nil {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			return
		}
		if _, ok := allowedQualities[quality]; !ok {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			return
		}

		session, err = b.ytSessions.UpdateQuality(ctx, query.From.ID, quality)
		if err != nil || session == nil {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText(locale.Get("youtube.session_expired", lang, nil)).WithShowAlert())
			return
		}

		chat := query.Message.GetChat()
		if linkparser.IsYouTubeShorts(session.URL) {
			_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
				ChatID:    tu.ID(chat.ID),
				MessageID: query.Message.GetMessageID(),
				Text:      youtube.ProcessingMessage(session.Lang, quality, "9_16"),
			})
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			b.startYouTubeDownload(ctx, bot, *session, "9_16")
			return
		}

		_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
			ChatID:    tu.ID(chat.ID),
			MessageID: query.Message.GetMessageID(),
			Text:      locale.Get("youtube.choose_ratio", session.Lang, nil),
			ReplyMarkup: youtube.RatioKeyboardFrom(session.Lang, b.runtime.CurrentStringList(ctx, "youtube.allowed_ratios", []string{"16_9", "21_9", "9_16"})),
		})
		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
	}
}

func (b *Bot) onRatioChoice(bot *telego.Bot) func(context.Context, *telego.Bot, telego.CallbackQuery) {
	return func(ctx context.Context, _ *telego.Bot, query telego.CallbackQuery) {
		if query.From.ID == 0 || query.Message == nil {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			return
		}
		lang := b.userLang(ctx, query.From.ID)
		allowedRatios := youtube.RatioSetFromStrings(b.runtime.CurrentStringList(ctx, "youtube.allowed_ratios", []string{"16_9", "21_9", "9_16"}))
		session, err := b.ytSessions.Get(ctx, query.From.ID)
		if err != nil || session == nil || session.Quality == nil {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText(locale.Get("youtube.session_expired", lang, nil)).WithShowAlert())
			return
		}

		ratio := strings.TrimPrefix(query.Data, "ratio:")
		if _, ok := allowedRatios[ratio]; !ok {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			return
		}

		chat := query.Message.GetChat()
		_, _ = bot.EditMessageText(&telego.EditMessageTextParams{
			ChatID:    tu.ID(chat.ID),
			MessageID: query.Message.GetMessageID(),
			Text:      youtube.ProcessingMessage(session.Lang, *session.Quality, ratio),
		})
		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
		b.startYouTubeDownload(ctx, bot, *session, ratio)
	}
}

func (b *Bot) startYouTubeDownload(ctx context.Context, bot *telego.Bot, session youtube.PendingSession, aspectRatio string) {
	_ = b.ytSessions.Clear(ctx, session.UserID)
	status, err := bot.SendMessage(tu.Message(
		tu.ID(session.ChatID),
		locale.Get("download.downloading", session.Lang, nil),
	))
	if err != nil {
		return
	}
	b.enqueueYouTube(ctx, bot, session.UserID, session.ChatID, status.MessageID, session.Lang, session.URL, *session.Quality, aspectRatio)
}

func (b *Bot) enqueueYouTube(ctx context.Context, bot *telego.Bot, userID, chatID int64, messageID int, lang, url string, quality int, aspectRatio string) {
	payload := queue.DownloadPayload{
		URL:         url,
		Platform:    "youtube",
		ChatID:      chatID,
		UserID:      userID,
		MessageID:   messageID,
		Lang:        lang,
		Quality:     quality,
		AspectRatio: aspectRatio,
		FormatID:    youtube.BuildFormat(quality, aspectRatio),
	}
	if err := b.q.EnqueueDownload(payload); err != nil {
		slog.Warn("enqueue youtube failed", "err", err)
		_, _ = bot.SendMessage(tu.Message(tu.ID(chatID), locale.Get("errors.generic", lang, nil)))
		return
	}
	metrics.DownloadsEnqueued.WithLabelValues("youtube").Inc()
}

func parseAutoQuality(raw string, allowed map[int]struct{}) *int {
	if raw == "" || raw == "ask" {
		return nil
	}
	quality, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	if _, ok := allowed[quality]; !ok {
		return nil
	}
	return &quality
}
