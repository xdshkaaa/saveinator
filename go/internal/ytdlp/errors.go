package ytdlp

import "strings"

func IsNoVideoFormatsError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no video formats found")
}

// IsNetworkError reports that the failure is a transient network-level
// issue (connection refused, no such host, connection reset, etc.) and
// therefore worth one more attempt.
func IsNetworkError(err error) bool {
	return err != nil && UserFacingErrorKey("", err) == "errors.network"
}

// IsTimeoutError reports that the failure is a timeout (context deadline
// exceeded or yt-dlp "timed out") and is therefore worth one more attempt.
func IsTimeoutError(err error) bool {
	return err != nil && UserFacingErrorKey("", err) == "download.timeout"
}

// IsRetryableError reports whether an error is transient enough to
// warrant a second attempt: network-level failures and timeouts.
// NOTE: TikTok "Unexpected response from webpage request" is intentionally
// NOT retryable via asynq — with MaxRetry(1) it would take 2× the download
// timeout and leave the user's placeholder message hanging ("вечно весит
// загрузка") until retry exhausted with no EditMessage. The transient case
// is handled as errors.format_unavailable with immediate user feedback.
func IsRetryableError(err error) bool {
	return IsNetworkError(err) || IsTimeoutError(err)
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
		// TikTok's webapp video-detail API answered with a status code (e.g.
		// 10240) instead of item data — the post is deterministically refused
		// (removed or audience-restricted), so a retry will not help. Verified
		// 2026-08: mirror APIs and the embed page refuse the same posts.
		strings.Contains(msg, "video not available, status code"),
		// TikTok private post/account (statusCode 10216/10222).
		strings.Contains(msg, "permission to view this post"),
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
