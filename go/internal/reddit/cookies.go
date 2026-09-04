package reddit

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// LoadCookieHeader reads a Netscape cookie file and renders its reddit.com
// cookies as a single "name=value; ..." Cookie header value. Reddit started
// requiring authentication for its public JSON endpoints (403 otherwise),
// so thread fetches must ride the same session as yt-dlp downloads.
// Missing or unreadable files yield an empty header, not an error.
func LoadCookieHeader(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	var pairs []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		// Cookie-Editor marks HttpOnly cookies as "#HttpOnly_..." — these are
		// real entries, unlike plain "#" comment lines.
		if strings.HasPrefix(line, "#HttpOnly_") {
			line = strings.TrimPrefix(line, "#HttpOnly_")
		} else if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		domain, name, value := fields[0], fields[5], fields[6]
		if !strings.HasSuffix(domain, "reddit.com") || name == "" {
			continue
		}
		if expires, err := strconv.ParseInt(fields[4], 10, 64); err == nil &&
			expires > 0 && expires < time.Now().Unix() {
			continue
		}
		pairs = append(pairs, name+"="+value)
	}
	return strings.Join(pairs, "; ")
}
