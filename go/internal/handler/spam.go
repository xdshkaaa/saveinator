package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/mymmrac/telego"

	"saveinator/internal/linkparser"
	"saveinator/internal/metrics"
)

func (b *Bot) allowGroupLinks(ctx context.Context, msg telego.Message) bool {
	body := messageBody(msg)
	if msg.Chat.Type == "private" || body == "" {
		return true
	}

	links := linkparser.ExtractURLs(body)
	if len(links) == 0 {
		return true
	}

	window := time.Duration(b.cfg.SpamDedupWindowSeconds) * time.Second
	if window <= 0 {
		window = 5 * time.Minute
	}

	for _, link := range links {
		sum := sha256.Sum256([]byte(link.URL))
		urlHash := hex.EncodeToString(sum[:])

		banned, err := b.db.IsLinkBanned(ctx, urlHash)
		if err != nil {
			slog.Warn("banned link check failed", "err", err)
		} else if banned {
			metrics.SpamBlocked.WithLabelValues("banned").Inc()
			return false
		}

		ok, err := b.redis.AllowURLDedup(ctx, urlHash, window)
		if err != nil {
			slog.Warn("url dedup check failed", "err", err)
			continue
		}
		if !ok {
			metrics.SpamBlocked.WithLabelValues("dedup").Inc()
			return false
		}
	}
	return true
}
