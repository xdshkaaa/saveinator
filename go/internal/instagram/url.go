package instagram

import (
	"regexp"
	"strings"
)

type Kind string

const (
	KindPost  Kind = "post"
	KindReel  Kind = "reel"
	KindStory Kind = "story"
	KindUnknown Kind = "unknown"
)

var shortcodeRe = regexp.MustCompile(`(?i)(?:instagram\.com|instagr\.am)/(?:p|reels?|tv|share/(?:reel|p))/([\w-]+)`)

func KindFromURL(url string) Kind {
	lower := strings.ToLower(url)
	switch {
	case strings.Contains(lower, "/stories/"):
		return KindStory
	case strings.Contains(lower, "/reel/"), strings.Contains(lower, "/reels/"), strings.Contains(lower, "/share/reel/"):
		return KindReel
	case strings.Contains(lower, "/p/"), strings.Contains(lower, "/share/p/"), strings.Contains(lower, "/tv/"):
		return KindPost
	default:
		return KindUnknown
	}
}

func IsPhotoPostURL(url string) bool {
	return KindFromURL(url) == KindPost
}

func ExtractShortcode(url string) string {
	if match := shortcodeRe.FindStringSubmatch(url); len(match) > 1 {
		return match[1]
	}
	return ""
}

func AllowSettingKey(kind Kind) string {
	switch kind {
	case KindPost:
		return "instagram.allow_posts"
	case KindReel:
		return "instagram.allow_reels"
	case KindStory:
		return "instagram.allow_stories"
	default:
		return ""
	}
}
