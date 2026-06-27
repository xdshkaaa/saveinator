package soundcloud

import (
	"regexp"
	"strings"
)

type LinkType string

const (
	LinkTypeTrack    LinkType = "track"
	LinkTypePlaylist LinkType = "playlist"
	LinkTypeShort    LinkType = "short"
)

var (
	discoverPlaylist = regexp.MustCompile(`(?i)(?:https?://)?(?:www\.)?soundcloud\.com/discover/sets/[^\s?#]+`)
	playlistURL      = regexp.MustCompile(`(?i)(?:https?://)?(?:www\.)?soundcloud\.com/[\w.-]+/sets/[^\s?#]+`)
	trackURL         = regexp.MustCompile(`(?i)(?:https?://)?(?:www\.)?soundcloud\.com/[\w.-]+/[\w.-]+`)
	shortURL         = regexp.MustCompile(`(?i)(?:https?://)?on\.soundcloud\.com/[\w-]+`)
)

type Link struct {
	Type LinkType
	URL  string
}

func ParseLink(raw string) (*Link, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, ErrInvalidURL
	}
	for _, pair := range []struct {
		re   *regexp.Regexp
		typ  LinkType
	}{
		{discoverPlaylist, LinkTypePlaylist},
		{playlistURL, LinkTypePlaylist},
		{shortURL, LinkTypeShort},
		{trackURL, LinkTypeTrack},
	} {
		if m := pair.re.FindString(text); m != "" {
			if pair.typ == LinkTypeTrack && strings.Contains(strings.ToLower(m), "/sets/") {
				continue
			}
			return &Link{Type: pair.typ, URL: normalizeURL(m)}, nil
		}
	}
	return nil, ErrInvalidURL
}

func normalizeURL(url string) string {
	if i := strings.Index(url, "?"); i >= 0 {
		url = url[:i]
	}
	if i := strings.Index(url, "#"); i >= 0 {
		url = url[:i]
	}
	return strings.TrimRight(url, "/")
}
