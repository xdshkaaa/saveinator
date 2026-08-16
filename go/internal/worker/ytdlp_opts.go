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

	instagramCookies := h.cfg.InstagramCookiesPath
	if strings.TrimSpace(h.cfg.InstagramCookiesFromBrowser) == "" {
		if synced := cookies.SyncFromMount(instagramCookies, cookies.InstagramWritablePath); synced != "" {
			instagramCookies = synced
		}
	}

	return ytdlp.Options{
		FormatID:                    formatID,
		Platform:                    platform,
		TikTokCookies:               tikTokCookies,
		TikTokCookiesFromBrowser:    h.cfg.TikTokCookiesFromBrowser,
		InstagramCookies:            instagramCookies,
		InstagramCookiesFromBrowser: h.cfg.InstagramCookiesFromBrowser,
		Referer:                     refererForPlatform(platform, h.cfg.TikTokReferer),
		Timeout:                     timeout,
	}
}

// refererForPlatform returns the Referer header value to send for a platform.
// TikTok's CDN blocks requests without a Referer ("Unexpected response from
// webpage request"); other platforms need none.
func refererForPlatform(platform, tiktokReferer string) string {
	if platform == "tiktok" {
		return tiktokReferer
	}
	return ""
}
