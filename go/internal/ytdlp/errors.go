package ytdlp

import "strings"

func IsNoVideoFormatsError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no video formats found")
}

// IsFormatUnavailableError reports the failure worth one more extraction
// attempt: yt-dlp resolved the video but no requested format survived. The
// usual cause is transient — the only YouTube player client that serves https
// formats without a PO token is intermittently refused on datacenter IPs, and
// the clients left over answer with storyboards, DRM or a captcha.
func IsFormatUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "requested format is not available") ||
		strings.Contains(msg, "only images are available") ||
		strings.Contains(msg, "po token") ||
		strings.Contains(msg, "drm protected") ||
		strings.Contains(msg, "captcha")
}

// IsUnexpectedWebpageError reports the TikTok extractor failing to parse the
// page TikTok served in response to a bot-challenge: "Unexpected response from
// webpage request". The video is fine — TikTok's CDN blocked the request
// (typically because no Referer header was sent). Retrying (ideally with a
// referer) usually works.
func IsUnexpectedWebpageError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unexpected response from webpage request")
}

// UserFacingErrorKey classifies a download error into a locale key more
// specific than the generic fallback, based on common yt-dlp and network
// failure signatures. Returns "" when nothing matches, so the caller can
// fall back to its own generic message.
func UserFacingErrorKey(platform string, err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "context deadline exceeded"),
		strings.Contains(msg, "timed out"):
		return "download.timeout"
	// Ahead of not_found on purpose: the captcha challenge arrives worded as
	// "Video unavailable", but the video is fine and retrying often works —
	// telling the user it was removed would be plainly wrong. Same for the
	// TikTok bot-challenge page ("Unexpected response from webpage request").
	case IsFormatUnavailableError(err), IsUnexpectedWebpageError(err):
		return "errors.format_unavailable"
	case strings.Contains(msg, "no video formats found"),
		strings.Contains(msg, "no video file found"),
		strings.Contains(msg, "no media files found"),
		strings.Contains(msg, "unsupported url"),
		strings.Contains(msg, "video unavailable"),
		strings.Contains(msg, "private video"),
		strings.Contains(msg, "login required"),
		strings.Contains(msg, "sign in to confirm"),
		strings.Contains(msg, "content isn't available"),
		strings.Contains(msg, "has been removed"):
		return "errors.not_found"
	case strings.Contains(msg, "429"),
		strings.Contains(msg, "too many requests"),
		strings.Contains(msg, "rate limit"),
		strings.Contains(msg, "rate-limit"):
		return "errors.rate_limited"
	case strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "no such host"),
		strings.Contains(msg, "network is unreachable"),
		strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "tls handshake"),
		strings.Contains(msg, "connection reset"):
		return "errors.network"
	default:
		return ""
	}
}
