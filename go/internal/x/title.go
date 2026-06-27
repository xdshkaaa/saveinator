package x

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	tcoURLRe         = regexp.MustCompile(`(?i)(?:\s*[-–—]\s*)?https?://t\.co/[A-Za-z0-9]+`)
	trailingMediaIDRe = regexp.MustCompile(`_\d{15,20}$`)
)

// DisplayTitle extracts a human-readable title from a yt-dlp X/Twitter output filename.
// Files follow %(title).100s_%(id)s.%(ext)s where title often includes author and t.co links.
func DisplayTitle(path string) string {
	base := strings.TrimSuffix(mediaBasename(path), filepath.Ext(path))
	base = trailingMediaIDRe.ReplaceAllString(base, "")
	return CleanRawTitle(base)
}

func mediaBasename(path string) string {
	dir := filepath.Dir(path)
	for dir != "." && dir != "/" {
		name := filepath.Base(dir)
		if strings.HasPrefix(name, "saveinator-") {
			return strings.TrimPrefix(path, dir+string(filepath.Separator))
		}
		dir = filepath.Dir(dir)
	}
	return filepath.Base(path)
}

// CleanRawTitle strips t.co URLs and trailing separators from tweet text or yt-dlp titles.
func CleanRawTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	title = tcoURLRe.ReplaceAllString(title, "")
	title = strings.TrimSpace(title)
	title = strings.TrimRight(title, "-–— ")
	return strings.TrimSpace(title)
}
