package youtube

import (
	"path/filepath"
	"strings"
)

// DisplayTitle extracts a human-readable title from a yt-dlp output filename.
// Files follow %(title).100s_%(id)s[_(aspect)].mp4 after optional transcode.
func DisplayTitle(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	for _, ratio := range []string{"9_16", "16_9", "21_9"} {
		if strings.HasSuffix(base, "_"+ratio) {
			base = strings.TrimSuffix(base, "_"+ratio)
			break
		}
	}
	if idx := strings.LastIndex(base, "_"); idx > 0 {
		candidate := base[idx+1:]
		if isVideoID(candidate) {
			base = base[:idx]
		}
	}
	return strings.TrimRight(strings.TrimSpace(base), "_")
}

func isVideoID(s string) bool {
	if len(s) != 11 {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}
