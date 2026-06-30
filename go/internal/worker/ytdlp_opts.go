package worker

import (
	"strings"
	"time"

	"saveinator/internal/cookies"
	"saveinator/internal/ytdlp"
)

func (h *Handler) ytdlpOpts(platform, formatID string, timeout time.Duration) ytdlp.Options {
	tikTokCookies := h.cfg.TikTokCookiesPath
	if strings.TrimSpace(h.cfg.TikTokCookiesFromBrowser) == "" {
		if synced := cookies.SyncFromMount(tikTokCookies, cookies.TikTokWritablePath); synced != "" {
			tikTokCookies = synced
		}
	}

	return ytdlp.Options{
		FormatID:                 formatID,
		Platform:                 platform,
		TikTokCookies:            tikTokCookies,
		TikTokCookiesFromBrowser: h.cfg.TikTokCookiesFromBrowser,
		Timeout:                  timeout,
	}
}
