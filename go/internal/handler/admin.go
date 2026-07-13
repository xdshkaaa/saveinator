package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/db"
	"saveinator/internal/locale"
	"saveinator/internal/metrics"
	"saveinator/internal/runtime"
)

func (b *Bot) onAdmin(bot *telego.Bot) func(context.Context, *telego.Bot, telego.Message) {
	return func(ctx context.Context, _ *telego.Bot, msg telego.Message) {
		if msg.From == nil || !b.isAdmin(msg.From.ID) {
			return
		}
		metrics.RecordCommand("admin")
		b.fsm.Clear(msg.From.ID)
		lang := b.userLang(ctx, msg.From.ID)
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("admin.menu_title", lang, nil)).
			WithParseMode(telego.ModeHTML).
			WithReplyMarkup(b.adminMainKeyboard(lang)))
	}
}

func (b *Bot) onStats(bot *telego.Bot) func(context.Context, *telego.Bot, telego.Message) {
	return func(ctx context.Context, _ *telego.Bot, msg telego.Message) {
		if msg.From == nil || !b.isAdmin(msg.From.ID) {
			return
		}
		metrics.RecordCommand("stats")
		b.fsm.Clear(msg.From.ID)
		lang := b.userLang(ctx, msg.From.ID)
		text, _ := b.renderStats(ctx, lang)
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), text).WithParseMode(telego.ModeHTML).WithReplyMarkup(b.statsKeyboard(lang)))
	}
}

func (b *Bot) onAdminCallback(bot *telego.Bot) func(context.Context, *telego.Bot, telego.CallbackQuery) {
	return func(ctx context.Context, _ *telego.Bot, query telego.CallbackQuery) {
		if query.From.ID == 0 || !b.isAdmin(query.From.ID) {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			return
		}
		if query.Message == nil {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			return
		}

		lang := b.userLang(ctx, query.From.ID)
		parts := strings.Split(query.Data, "|")
		if len(parts) < 2 || parts[0] != "admin" {
			_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
			return
		}

		chatID := query.Message.GetChat().ID
		msgID := query.Message.GetMessageID()

		switch parts[1] {
		case "menu":
			b.fsm.Clear(query.From.ID)
			b.editAdminText(bot, chatID, msgID, locale.Get("admin.menu_title", lang, nil), b.adminMainKeyboard(lang))
		case "svc":
			if len(parts) < 3 {
				break
			}
			b.fsm.Clear(query.From.ID)
			service := parts[2]
			text, _ := b.serviceSummary(ctx, service, lang)
			b.editAdminText(bot, chatID, msgID, text, b.serviceKeyboard(service, lang))
		case "bans":
			b.fsm.Clear(query.From.ID)
			b.showBans(ctx, bot, chatID, msgID, lang)
		case "ban":
			if len(parts) >= 3 && parts[2] == "add" {
				b.fsm.Set(query.From.ID, "admin_ban", nil)
				b.editAdminText(bot, chatID, msgID, locale.Get("admin.enter_ban_id", lang, nil), b.backToBansKeyboard(lang))
			}
		case "unban":
			if len(parts) >= 3 {
				uid, _ := strconv.ParseInt(parts[2], 10, 64)
				removed, _ := b.redis.UnbanUser(ctx, uid)
				toast := locale.Get("admin.unban_done", lang, map[string]string{"user_id": parts[2]})
				if !removed {
					toast = locale.Get("admin.unban_not_found", lang, map[string]string{"user_id": parts[2]})
				}
				b.showBans(ctx, bot, chatID, msgID, lang)
				_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText(toast))
				return
			}
		case "stats":
			text, _ := b.renderStats(ctx, lang)
			b.editAdminText(bot, chatID, msgID, text, b.statsKeyboard(lang))
		case "broadcasts":
			b.fsm.Clear(query.From.ID)
			b.editAdminText(bot, chatID, msgID, locale.Get("broadcast.menu_title", lang, nil), b.broadcastMenuKeyboard(lang))
		case "edit":
			if len(parts) < 3 {
				break
			}
			b.adminStartEdit(bot, query, parts[2], lang)
		case "edit_bool":
			if len(parts) < 3 {
				break
			}
			b.adminToggleBool(ctx, bot, query, parts[2], lang)
			return
		case "set_enum":
			if len(parts) < 4 {
				break
			}
			b.adminSetEnum(ctx, bot, query, parts[2], parts[3], lang)
			return
		case "confirm":
			b.adminConfirm(ctx, bot, query, parts, lang)
			return
		case "reset":
			b.adminReset(ctx, bot, query, parts, lang)
			return
		}
		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
	}
}

func (b *Bot) adminMainKeyboard(lang string) *telego.InlineKeyboardMarkup {
	var rows [][]telego.InlineKeyboardButton
	var row []telego.InlineKeyboardButton
	for i, svc := range runtime.ServiceOrder {
		row = append(row, tu.InlineKeyboardButton(runtime.ServiceLabel(svc, lang)).WithCallbackData("admin|svc|"+svc))
		if len(row) == 2 {
			rows = append(rows, row)
			row = nil
		}
		if i == len(runtime.ServiceOrder)-1 && len(row) > 0 {
			rows = append(rows, row)
		}
	}
	rows = append(rows, []telego.InlineKeyboardButton{
		tu.InlineKeyboardButton(locale.Get("admin.btn_global", lang, nil)).WithCallbackData("admin|svc|global"),
	})
	rows = append(rows, []telego.InlineKeyboardButton{
		tu.InlineKeyboardButton(locale.Get("admin.btn_broadcasts", lang, nil)).WithCallbackData("admin|broadcasts"),
		tu.InlineKeyboardButton(locale.Get("admin.btn_users", lang, nil)).WithCallbackData("admin|stats"),
	})
	rows = append(rows, []telego.InlineKeyboardButton{
		tu.InlineKeyboardButton(locale.Get("admin.btn_bans", lang, nil)).WithCallbackData("admin|bans"),
		tu.InlineKeyboardButton(locale.Get("admin.btn_reset_all", lang, nil)).WithCallbackData("admin|confirm|reset_all"),
	})
	return tu.InlineKeyboard(rows...)
}

func (b *Bot) serviceKeyboard(service, lang string) *telego.InlineKeyboardMarkup {
	var rows [][]telego.InlineKeyboardButton
	for _, def := range runtime.ServiceSettings(service) {
		label := runtime.KindLabel(def, lang)
		switch def.ValueType {
		case runtime.TypeBool:
			rows = append(rows, tu.InlineKeyboardRow(
				tu.InlineKeyboardButton(locale.Get("admin.btn_change", lang, map[string]string{"label": label})).
					WithCallbackData("admin|edit_bool|"+def.RedisKey),
			))
		case runtime.TypeEnum:
			for _, val := range def.Allowed {
				btnLabel := fmt.Sprintf("%s → %s", label, val)
				rows = append(rows, tu.InlineKeyboardRow(
					tu.InlineKeyboardButton(btnLabel).WithCallbackData("admin|set_enum|"+def.RedisKey+"|"+val),
				))
			}
		default:
			rows = append(rows, tu.InlineKeyboardRow(
				tu.InlineKeyboardButton(locale.Get("admin.btn_change", lang, map[string]string{"label": label})).
					WithCallbackData("admin|edit|"+def.RedisKey),
			))
		}
	}
	rows = append(rows, tu.InlineKeyboardRow(
		tu.InlineKeyboardButton(locale.Get("admin.btn_reset_service", lang, nil)).WithCallbackData("admin|confirm|reset_svc|"+service),
	))
	rows = append(rows, tu.InlineKeyboardRow(
		tu.InlineKeyboardButton(locale.Get("admin.btn_back", lang, nil)).WithCallbackData("admin|menu"),
	))
	return tu.InlineKeyboard(rows...)
}

func (b *Bot) statsKeyboard(lang string) *telego.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(locale.Get("admin.btn_stats_refresh", lang, nil)).WithCallbackData("admin|stats|refresh"),
			tu.InlineKeyboardButton(locale.Get("admin.btn_back", lang, nil)).WithCallbackData("admin|menu"),
		),
	)
}

func (b *Bot) backToBansKeyboard(lang string) *telego.InlineKeyboardMarkup {
	return tu.InlineKeyboard(tu.InlineKeyboardRow(
		tu.InlineKeyboardButton(locale.Get("admin.btn_back", lang, nil)).WithCallbackData("admin|bans"),
	))
}

func (b *Bot) serviceSummary(ctx context.Context, service, lang string) (string, error) {
	lines := []string{locale.Get("admin.service_title", lang, map[string]string{"service": runtime.ServiceLabel(service, lang)})}
	defs := runtime.ServiceSettings(service)
	if len(defs) == 0 {
		lines = append(lines, locale.Get("admin.no_settings", lang, nil))
	} else {
		for _, def := range defs {
			val, _ := b.runtime.CurrentValue(ctx, def)
			lines = append(lines, locale.Get("admin.setting_line", lang, map[string]string{
				"label": runtime.KindLabel(def, lang),
				"value": runtime.FormatValue(val, def, lang),
			}))
		}
	}
	lines = append(lines, locale.Get("admin.hot_swap_hint", lang, nil))
	return strings.Join(lines, "\n"), nil
}

func (b *Bot) showBans(ctx context.Context, bot *telego.Bot, chatID int64, msgID int, lang string) {
	ids, _ := b.redis.ListBannedUsers(ctx)
	lines := []string{locale.Get("admin.bans_title", lang, nil)}
	if len(ids) == 0 {
		lines = append(lines, locale.Get("admin.bans_empty", lang, nil))
	} else {
		for _, id := range ids {
			lines = append(lines, locale.Get("admin.ban_user_line", lang, map[string]string{
				"user_id": strconv.FormatInt(id, 10),
			}))
		}
	}
	lines = append(lines, locale.Get("admin.bans_hint", lang, nil))
	b.editAdminText(bot, chatID, msgID, strings.Join(lines, "\n"), b.bansKeyboard(lang, ids))
}

func (b *Bot) bansKeyboard(lang string, ids []int64) *telego.InlineKeyboardMarkup {
	var rows [][]telego.InlineKeyboardButton
	for _, id := range ids {
		rows = append(rows, tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(locale.Get("admin.btn_unban_user", lang, map[string]string{
				"user_id": strconv.FormatInt(id, 10),
			})).WithCallbackData(fmt.Sprintf("admin|unban|%d", id)),
		))
	}
	rows = append(rows, tu.InlineKeyboardRow(
		tu.InlineKeyboardButton(locale.Get("admin.btn_ban_add", lang, nil)).WithCallbackData("admin|ban|add"),
	))
	rows = append(rows, tu.InlineKeyboardRow(
		tu.InlineKeyboardButton(locale.Get("admin.btn_back", lang, nil)).WithCallbackData("admin|menu"),
	))
	return tu.InlineKeyboard(rows...)
}

func (b *Bot) adminStartEdit(bot *telego.Bot, query telego.CallbackQuery, redisKey, lang string) {
	def, ok := runtime.Lookup(redisKey)
	if !ok {
		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
		return
	}
	b.fsm.Set(query.From.ID, "admin_edit", map[string]string{"redis_key": redisKey, "service": def.Service})
	val, _ := b.runtime.CurrentValue(context.Background(), def)
	hint := ""
	if def.ValueType == runtime.TypeInt && (def.MinValue > 0 || def.MaxValue > 0) {
		hint = fmt.Sprintf("\n(min=%d, max=%d)", def.MinValue, def.MaxValue)
	}
	if def.ValueType == runtime.TypeList && len(def.Allowed) > 0 {
		hint += fmt.Sprintf("\n(allowed: %s)", strings.Join(def.Allowed, ", "))
	}
	text := locale.Get("admin.enter_value", lang, map[string]string{
		"service": runtime.ServiceLabel(def.Service, lang),
		"label":   runtime.KindLabel(def, lang),
		"current": runtime.FormatValue(val, def, lang),
	}) + hint
	chat := query.Message.GetChat()
	b.editAdminText(bot, chat.ID, query.Message.GetMessageID(), text, tu.InlineKeyboard(
		tu.InlineKeyboardRow(tu.InlineKeyboardButton(locale.Get("admin.btn_back", lang, nil)).WithCallbackData("admin|svc|"+def.Service)),
	))
	_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
}

func (b *Bot) adminSaveEdit(ctx context.Context, bot *telego.Bot, msg telego.Message, lang, redisKey, raw string) bool {
	def, ok := runtime.Lookup(redisKey)
	if !ok {
		b.fsm.Clear(msg.From.ID)
		return true
	}
	if err := b.runtime.Validate(def, raw); err != nil {
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("admin.invalid_value", lang, map[string]string{"error": err.Error()})))
		return true
	}
	parsed := b.runtime.Parse(def, raw)
	_ = b.runtime.SetValue(ctx, redisKey, parsed)
	metrics.RecordAdminRuntimeSetting(def.Service)
	b.fsm.Clear(msg.From.ID)
	_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("admin.saved", lang, map[string]string{
		"label": runtime.KindLabel(def, lang),
		"value": runtime.FormatValue(parsed, def, lang),
	})))
	summary, _ := b.serviceSummary(ctx, def.Service, lang)
	_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), summary).WithReplyMarkup(b.serviceKeyboard(def.Service, lang)))
	return true
}

func (b *Bot) adminSaveBan(ctx context.Context, bot *telego.Bot, msg telego.Message, lang, raw string) bool {
	uid, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || uid <= 0 {
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("admin.invalid_user_id", lang, nil)))
		return true
	}
	if uid == msg.From.ID {
		_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("admin.cannot_ban_self", lang, nil)))
		return true
	}
	_ = b.redis.BanUser(ctx, uid)
	b.fsm.Clear(msg.From.ID)
	_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("admin.ban_added", lang, map[string]string{
		"user_id": strconv.FormatInt(uid, 10),
	})))
	return true
}

func (b *Bot) adminSetEnum(ctx context.Context, bot *telego.Bot, query telego.CallbackQuery, redisKey, value, lang string) {
	def, ok := runtime.Lookup(redisKey)
	if !ok || def.ValueType != runtime.TypeEnum {
		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
		return
	}
	if err := b.runtime.Validate(def, value); err != nil {
		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText(locale.Get("admin.invalid_value", lang, map[string]string{"error": err.Error()})).WithShowAlert())
		return
	}
	_ = b.runtime.SetValue(ctx, redisKey, value)
	metrics.RecordAdminRuntimeSetting(def.Service)
	text, _ := b.serviceSummary(ctx, def.Service, lang)
	chat := query.Message.GetChat()
	b.editAdminText(bot, chat.ID, query.Message.GetMessageID(), text, b.serviceKeyboard(def.Service, lang))
	_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText(locale.Get("admin.saved", lang, map[string]string{
		"label": runtime.KindLabel(def, lang),
		"value": value,
	})))
}

func (b *Bot) adminToggleBool(ctx context.Context, bot *telego.Bot, query telego.CallbackQuery, redisKey, lang string) {
	def, ok := runtime.Lookup(redisKey)
	if !ok {
		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
		return
	}
	current, _ := b.runtime.CurrentValue(ctx, def)
	cur, _ := current.(bool)
	_ = b.runtime.SetValue(ctx, redisKey, !cur)
	metrics.RecordAdminRuntimeSetting(def.Service)
	text, _ := b.serviceSummary(ctx, def.Service, lang)
	chat := query.Message.GetChat()
	b.editAdminText(bot, chat.ID, query.Message.GetMessageID(), text, b.serviceKeyboard(def.Service, lang))
	_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID).WithText(locale.Get("admin.reset_done_toast", lang, nil)))
}

func (b *Bot) adminConfirm(ctx context.Context, bot *telego.Bot, query telego.CallbackQuery, parts []string, lang string) {
	if len(parts) < 3 {
		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
		return
	}
	chat := query.Message.GetChat()
	msgID := query.Message.GetMessageID()

	switch parts[2] {
	case "reset_all":
		b.editAdminText(bot, chat.ID, msgID, locale.Get("admin.confirm_reset_all", lang, nil), tu.InlineKeyboard(
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton(locale.Get("admin.confirm_yes", lang, nil)).WithCallbackData("admin|confirm|reset_all_yes"),
				tu.InlineKeyboardButton(locale.Get("admin.confirm_no", lang, nil)).WithCallbackData("admin|menu"),
			),
		))
	case "reset_all_yes":
		_ = b.runtime.ResetAll(ctx)
		b.fsm.Clear(query.From.ID)
		b.editAdminText(bot, chat.ID, msgID, locale.Get("admin.reset_all_done", lang, nil), b.adminMainKeyboard(lang))
	case "reset_svc":
		if len(parts) < 4 {
			break
		}
		service := parts[3]
		b.editAdminText(bot, chat.ID, msgID, locale.Get("admin.confirm_reset_service", lang, map[string]string{
			"service": runtime.ServiceLabel(service, lang),
		}), tu.InlineKeyboard(
			tu.InlineKeyboardRow(
				tu.InlineKeyboardButton(locale.Get("admin.confirm_yes", lang, nil)).WithCallbackData("admin|confirm|reset_svc_yes|"+service),
				tu.InlineKeyboardButton(locale.Get("admin.confirm_no", lang, nil)).WithCallbackData("admin|svc|"+service),
			),
		))
	case "reset_svc_yes":
		if len(parts) < 4 {
			break
		}
		service := parts[3]
		for _, def := range runtime.ServiceSettings(service) {
			_ = b.runtime.Reset(ctx, def.RedisKey)
		}
		text, _ := b.serviceSummary(ctx, service, lang)
		b.editAdminText(bot, chat.ID, msgID, text, b.serviceKeyboard(service, lang))
	}
	_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
}

func (b *Bot) adminReset(ctx context.Context, bot *telego.Bot, query telego.CallbackQuery, parts []string, lang string) {
	if len(parts) < 3 {
		_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
		return
	}
	chat := query.Message.GetChat()
	msgID := query.Message.GetMessageID()
	switch parts[1] + "|" + parts[2] {
	case "reset|all":
		_ = b.runtime.ResetAll(ctx)
		b.editAdminText(bot, chat.ID, msgID, locale.Get("admin.reset_all_done", lang, nil), b.adminMainKeyboard(lang))
	case "reset|svc":
		if len(parts) >= 4 {
			for _, def := range runtime.ServiceSettings(parts[3]) {
				_ = b.runtime.Reset(ctx, def.RedisKey)
			}
			text, _ := b.serviceSummary(ctx, parts[3], lang)
			b.editAdminText(bot, chat.ID, msgID, text, b.serviceKeyboard(parts[3], lang))
		}
	}
	_ = bot.AnswerCallbackQuery(tu.CallbackQuery(query.ID))
}

func (b *Bot) renderStats(ctx context.Context, lang string) (string, error) {
	banned, _ := b.redis.BannedCount(ctx)
	stats, err := b.db.FetchUserStats(ctx, banned)
	if err != nil {
		slog.Warn("fetch stats failed", "err", err)
		return locale.Get("errors.generic", lang, nil), err
	}

	activeNow, _ := b.redis.CountActiveUsers(ctx, 30*time.Minute)

	var platformLines string
	if len(stats.TopPlatforms7d) == 0 {
		platformLines = locale.Get("admin.stats_platforms_empty", lang, nil)
	} else {
		var lines []string
		for _, p := range stats.TopPlatforms7d {
			lines = append(lines, locale.Get("admin.stats_platform_line", lang, map[string]string{
				"platform": runtime.ServiceLabel(db.FromDBPlatform(p.Platform), lang),
				"count":    strconv.Itoa(p.Count),
			}))
		}
		platformLines = strings.Join(lines, "\n")
	}

	downloadsLine := formatStatsDownloads(stats.DownloadsToday, stats.Downloads7d, stats.Downloads30d)
	if lang == "ru" {
		downloadsLine = formatStatsDownloadsRU(stats.DownloadsToday, stats.Downloads7d, stats.Downloads30d)
	}

	returningPct := formatPct(stats.ReturningUsers, stats.UsersWithDownloads)
	if stats.UsersWithDownloads > 0 {
		returningPct = locale.Get("admin.stats_returning_pct_suffix", lang, map[string]string{"pct": returningPct})
	}

	body := locale.Get("admin.stats_body", lang, map[string]string{
		"updated":          time.Now().UTC().Format("02.01.2006 15:04"),
		"total":            strconv.Itoa(stats.TotalUsers),
		"new_today":        strconv.Itoa(stats.NewToday),
		"growth_delta":     formatGrowthDelta(stats.NewToday, stats.NewYesterday, lang),
		"new_7d":           strconv.Itoa(stats.New7d),
		"new_30d":          strconv.Itoa(stats.New30d),
		"active_now":       strconv.Itoa(activeNow),
		"dau":              strconv.Itoa(stats.DAU),
		"wau":              strconv.Itoa(stats.WAU),
		"mau":              strconv.Itoa(stats.MAU),
		"stickiness":       formatStickiness(stats.DAU, stats.MAU),
		"downloads_line":   downloadsLine,
		"success_rate":     formatSuccessRate(stats.Completed30d, stats.Failed30d),
		"with_downloads":   strconv.Itoa(stats.UsersWithDownloads),
		"with_downloads_pct": formatPct(stats.UsersWithDownloads, stats.TotalUsers),
		"returning":        strconv.Itoa(stats.ReturningUsers),
		"returning_pct":    returningPct,
		"lang_line": locale.Get("admin.stats_lang_line", lang, map[string]string{
			"en":      strconv.Itoa(stats.LanguageEN),
			"en_pct":  formatPct(stats.LanguageEN, stats.TotalUsers),
			"ru":      strconv.Itoa(stats.LanguageRU),
			"ru_pct":  formatPct(stats.LanguageRU, stats.TotalUsers),
			"kk":      strconv.Itoa(stats.LanguageKK),
			"kk_pct":  formatPct(stats.LanguageKK, stats.TotalUsers),
		}),
		"platform_lines": platformLines,
		"banned":         strconv.Itoa(stats.BannedCount),
		"download_note":  locale.Get("admin.stats_download_note", lang, nil),
	})
	return locale.Get("admin.stats_title", lang, nil) + "\n\n" + body, nil
}

func (b *Bot) editAdminText(bot *telego.Bot, chatID int64, msgID int, text string, kb *telego.InlineKeyboardMarkup) {
	_, err := bot.EditMessageText(&telego.EditMessageTextParams{
		ChatID:      tu.ID(chatID),
		MessageID:   msgID,
		Text:        text,
		ParseMode:   telego.ModeHTML,
		ReplyMarkup: kb,
	})
	if err != nil {
		slog.Warn("edit admin message failed", "err", err)
	}
}
