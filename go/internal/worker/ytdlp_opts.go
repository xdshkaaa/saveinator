package worker

import (
	"time"

	"saveinator/internal/ytdlp"
)

func (h *Handler) ytdlpOpts(platform, formatID string, timeout time.Duration) ytdlp.Options {
	return ytdlp.Options{
		FormatID:                    formatID,
		Platform:                    platform,
		InstagramCookies:            h.cfg.InstagramCookiesPath,
		InstagramCookiesFromBrowser: h.cfg.InstagramCookiesFromBrowser,
		TikTokCookies:               h.cfg.TikTokCookiesPath,
		TikTokCookiesFromBrowser:    h.cfg.TikTokCookiesFromBrowser,
		Timeout:                     timeout,
	}
}
