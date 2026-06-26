package pinterest

import (
	"regexp"
	"strings"
)

type URLType string

const (
	URLTypePin    URLType = "pin"
	URLTypeBoard  URLType = "board"
	URLTypeShort  URLType = "short"
	URLTypeUnknown URLType = "unknown"
)

var (
	pinPattern   = regexp.MustCompile(`(?i)(?:https?://)?(?:[\w-]+\.)?pinterest\.com/pin/[\w-]+/?(?:[?&]\S*)?`)
	shortPattern = regexp.MustCompile(`(?i)(?:https?://)?pin\.it/[\w-]+/?(?:[?&]\S*)?`)
	boardPattern = regexp.MustCompile(`(?i)(?:https?://)?(?:[\w-]+\.)?pinterest\.com/[\w-]+/[\w-]+/?(?:[?&]\S*)?`)
	pinIDPattern = regexp.MustCompile(`(?i)pin/(\d+)`)

	reservedBoardSegments = map[string]struct{}{
		"pin": {}, "search": {}, "ideas": {}, "today": {},
		"shopping": {}, "videos": {}, "topics": {},
	}
)

type ParsedURL struct {
	URL     string
	URLType URLType
}

func ParseURL(raw string) (*ParsedURL, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return nil, ErrInvalidURL
	}

	if m := pinPattern.FindString(normalized); m != "" {
		return &ParsedURL{URL: m, URLType: URLTypePin}, nil
	}
	if m := shortPattern.FindString(normalized); m != "" {
		return &ParsedURL{URL: m, URLType: URLTypeShort}, nil
	}
	if m := boardPattern.FindString(normalized); m != "" {
		path := m
		if idx := strings.Index(strings.ToLower(path), "pinterest.com/"); idx >= 0 {
			rest := path[idx+len("pinterest.com/"):]
			segment := strings.SplitN(rest, "/", 2)[0]
			if _, reserved := reservedBoardSegments[strings.ToLower(segment)]; !reserved {
				return &ParsedURL{URL: m, URLType: URLTypeBoard}, nil
			}
		}
	}
	return nil, ErrInvalidURL
}

func ExtractPinID(url string) string {
	m := pinIDPattern.FindStringSubmatch(url)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}
