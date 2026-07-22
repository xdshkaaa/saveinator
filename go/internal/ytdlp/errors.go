package ytdlp

import "strings"

func IsNoVideoFormatsError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no video formats found")
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
