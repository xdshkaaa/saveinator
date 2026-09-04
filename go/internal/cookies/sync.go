package cookies

import (
	"os"
	"strings"
)

const (
	TikTokWritablePath    = "/tmp/tiktok_cookies.txt"
	InstagramWritablePath = "/tmp/instagram_cookies.txt"
	RedditWritablePath    = "/tmp/reddit_cookies.txt"
)

// SyncFromMount copies src to dst when dst is missing or src is newer than dst.
// Returns dst when a writable copy is available, otherwise src when readable.
func SyncFromMount(src, dst string) string {
	src = strings.TrimSpace(src)
	if src == "" {
		return ""
	}
	srcInfo, err := os.Stat(src)
	if err != nil || srcInfo.IsDir() || srcInfo.Size() == 0 {
		return ""
	}

	dstInfo, dstErr := os.Stat(dst)
	if dstErr == nil && !dstInfo.IsDir() && dstInfo.Size() > 0 {
		if !srcInfo.ModTime().After(dstInfo.ModTime()) {
			return dst
		}
	}

	data, err := os.ReadFile(src)
	if err != nil {
		if dstErr == nil && !dstInfo.IsDir() && dstInfo.Size() > 0 {
			return dst
		}
		return ""
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return src
	}
	return dst
}
