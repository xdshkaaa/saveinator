package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/locale"
)

func (b *Bot) checkBanned(ctx context.Context, bot *telego.Bot, msg telego.Message, lang string) bool {
	if msg.From == nil || b.isAdmin(msg.From.ID) {
		return false
	}
	banned, err := b.redis.IsUserBanned(ctx, msg.From.ID)
	if err != nil || !banned {
		return false
	}
	_, _ = bot.SendMessage(tu.Message(tu.ID(msg.Chat.ID), locale.Get("ban.shadow_reply", lang, nil)))
	if b.isAdmin(b.cfg.AdminTelegramID) {
		user := strconv.FormatInt(msg.From.ID, 10)
		if msg.From.Username != "" {
			user = "@" + msg.From.Username
		}
		notice := locale.Get("ban.admin_notice", "ru", map[string]string{
			"user": user,
			"chat": fmt.Sprintf("%d", msg.Chat.ID),
		})
		_, _ = bot.SendMessage(tu.Message(tu.ID(b.cfg.AdminTelegramID), notice))
		if msg.Text != "" {
			_, _ = bot.ForwardMessage(&telego.ForwardMessageParams{
				ChatID:     tu.ID(b.cfg.AdminTelegramID),
				FromChatID: tu.ID(msg.Chat.ID),
				MessageID:  msg.MessageID,
			})
		}
	}
	return true
}

func (b *Bot) handleAdminFSM(ctx context.Context, bot *telego.Bot, msg telego.Message, lang string) bool {
	if msg.From == nil || !b.isAdmin(msg.From.ID) {
		return false
	}
	state, ok := b.fsm.Get(msg.From.ID)
	if !ok {
		return false
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return true
	}

	switch state.Kind {
	case "admin_edit":
		return b.adminSaveEdit(ctx, bot, msg, lang, state.Data["redis_key"], text)
	case "admin_ban":
		return b.adminSaveBan(ctx, bot, msg, lang, text)
	case "broadcast_text":
		return b.broadcastSaveText(ctx, bot, msg, lang, text, 0)
	case "broadcast_edit":
		id, _ := strconv.Atoi(state.Data["broadcast_id"])
		return b.broadcastSaveText(ctx, bot, msg, lang, text, id)
	default:
		b.fsm.Clear(msg.From.ID)
		return false
	}
}
