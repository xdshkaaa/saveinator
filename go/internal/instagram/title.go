package instagram

import (
	"path/filepath"
	"strings"
)

// DisplayTitle extracts a human-readable title from a yt-dlp output filename.
// Files follow %(title).100s_%(id)s.%(ext)s where id is the Instagram shortcode.
func DisplayTitle(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if idx := strings.LastIndex(base, "_"); idx > 0 {
		candidate := base[idx+1:]
		if isShortcode(candidate) {
			base = base[:idx]
		}
	}
	base = strings.TrimRight(strings.TrimSpace(base), "_")
	return normalizeFallbackTitle(base)
}

func normalizeFallbackTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	for _, prefix := range []string{"video by ", "photo by ", "post by ", "reel by "} {
		if strings.HasPrefix(strings.ToLower(title), prefix) {
			return ""
		}
	}
	return title
}

func isShortcode(s string) bool {
	if len(s) < 8 || len(s) > 15 {
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
