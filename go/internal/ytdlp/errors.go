package ytdlp

import "strings"

func IsNoVideoFormatsError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no video formats found")
}

func IsEmptyMediaResponseError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "empty media response")
}

func IsInstagramPhotoFallbackError(err error) bool {
	return IsNoVideoFormatsError(err) || IsEmptyMediaResponseError(err)
}

func UserFacingErrorKey(platform string, err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "context deadline exceeded"),
		strings.Contains(msg, "timed out"):
		return "download.timeout"
	case platform == "instagram" && !strings.Contains(msg, "read-only file system") && (strings.Contains(msg, "use --cookies") ||
		strings.Contains(msg, "cookies-from-browser") ||
		(!strings.Contains(msg, "empty media response") && (strings.Contains(msg, "login") ||
			(strings.Contains(msg, "cookies") && !strings.Contains(msg, "yt_dlp/cookies.py")) ||
			strings.Contains(msg, "rate-limit") ||
			strings.Contains(msg, "checkpoint") ||
			strings.Contains(msg, "authentication")))):
		return "instagram.auth_required"
	case platform == "instagram":
		return "instagram.download_failed"
	default:
		return "download.timeout"
	}
}
