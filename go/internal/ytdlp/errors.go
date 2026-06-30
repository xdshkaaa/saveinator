package ytdlp

import "strings"

func IsNoVideoFormatsError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no video formats found")
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
	default:
		return "download.timeout"
	}
}
