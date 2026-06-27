package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/mymmrac/telego/telegoapi"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/queue"
)

func (h *Handler) handleBroadcast(ctx context.Context, t *asynq.Task) error {
	var p queue.BroadcastPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}

	bc, err := h.db.GetBroadcast(ctx, p.BroadcastID)
	if err != nil || bc == nil {
		return err
	}

	_ = h.db.UpdateBroadcastStatus(ctx, p.BroadcastID, "RUNNING", len(p.UserIDs))

	delayMS := h.runtime.CurrentInt(ctx, "global.broadcast_delay_ms", h.cfg.BroadcastDelayMS)
	batchSize := h.runtime.CurrentInt(ctx, "global.broadcast_batch_size", h.cfg.BroadcastBatchSize)
	delay := time.Duration(delayMS) * time.Millisecond

	sent, failed, blocked := 0, 0, 0
	total := len(p.UserIDs)

	for idx, userID := range p.UserIDs {
		sendErr := h.sendBroadcastMessage(userID, bc.Text)
		switch {
		case sendErr == nil:
			sent++
			_ = h.db.SaveBroadcastDelivery(ctx, p.BroadcastID, userID, "SENT", "")
		case isBlockedError(sendErr):
			blocked++
			_ = h.db.SaveBroadcastDelivery(ctx, p.BroadcastID, userID, "BLOCKED", "")
		default:
			if retryAfter := extractRetryAfter(sendErr); retryAfter > 0 {
				time.Sleep(time.Duration(retryAfter) * time.Second)
				if err2 := h.sendBroadcastMessage(userID, bc.Text); err2 == nil {
					sent++
					_ = h.db.SaveBroadcastDelivery(ctx, p.BroadcastID, userID, "SENT", "")
				} else {
					failed++
					_ = h.db.SaveBroadcastDelivery(ctx, p.BroadcastID, userID, "FAILED", truncateErr(err2))
				}
			} else {
				failed++
				_ = h.db.SaveBroadcastDelivery(ctx, p.BroadcastID, userID, "FAILED", truncateErr(sendErr))
			}
		}

		if batchSize > 0 && (idx+1)%batchSize == 0 {
			_ = h.db.UpdateBroadcastProgress(ctx, p.BroadcastID, sent, failed, blocked)
		}
		if delay > 0 && idx < total-1 {
			time.Sleep(delay)
		}
	}

	_ = h.db.CompleteBroadcast(ctx, p.BroadcastID, sent, failed, blocked)
	slog.Info("broadcast completed", "id", p.BroadcastID, "sent", sent, "total", total, "failed", failed, "blocked", blocked)
	return nil
}

func (h *Handler) sendBroadcastMessage(userID int64, text string) error {
	_, err := h.bot.SendMessage(tu.Message(tu.ID(userID), text))
	return err
}

func isBlockedError(err error) bool {
	var apiErr *telegoapi.Error
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode == 403
	}
	return strings.Contains(strings.ToLower(err.Error()), "forbidden")
}

func extractRetryAfter(err error) int {
	var apiErr *telegoapi.Error
	if errors.As(err, &apiErr) && apiErr.Parameters.RetryAfter > 0 {
		return apiErr.Parameters.RetryAfter
	}
	return 0
}

func truncateErr(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 500 {
		return s[:500]
	}
	return s
}
