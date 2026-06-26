package linkparser

import (
	"regexp"
	"strings"
)

type Platform string

const (
	PlatformYouTube    Platform = "youtube"
	PlatformTikTok     Platform = "tiktok"
	PlatformInstagram  Platform = "instagram"
	PlatformX          Platform = "x"
	PlatformSpotify    Platform = "spotify"
	PlatformSoundCloud Platform = "soundcloud"
	PlatformPinterest  Platform = "pinterest"
	PlatformUnknown    Platform = "unknown"
)

type ParsedLink struct {
	Platform   Platform
	URL        string
	XStatusID  string
	SpotifyID  string
	SpotifyTyp string
}

type pattern struct {
	platform Platform
	re       *regexp.Regexp
}

var (
	urlExtractor     = regexp.MustCompile(`(?i)https?://\S+`)
	xStatusIDRegex   = regexp.MustCompile(`/status/(\d+)`)
	youtubeShortsRe  = regexp.MustCompile(`(?i)youtube\.com/shorts/`)
	spotifyURIRegex  = regexp.MustCompile(`(?i)spotify:(?:album|track):([A-Za-z0-9]{22})`)
	spotifyIDInURL   = `[A-Za-z0-9]{22}`

	patterns = []pattern{
		{PlatformYouTube, regexp.MustCompile(`(?i)(?:https?://)?(?:(?:www\.|m\.)?youtube\.com/(?:watch\?v=|shorts/)|youtu\.be/)[\w-]{11}(?:[?&]\S*)?`)},
		{PlatformTikTok, regexp.MustCompile(`(?i)(?:https?://)?(?:` +
			`(?:www\.)?tiktok\.com/@[\w.-]*/(?:video|photo)/\d+(?:[/?#&]\S*)?` +
			`|(?:www\.)?tiktok\.com/t/[\w-]+/?(?:[?&]\S*)?` +
			`|(?:vm|vt)\.tiktok\.com/[\w-]+/?(?:[?&]\S*)?` +
			`|m\.tiktok\.com/v/\d+\.html(?:[?&]\S*)?` +
			`)`)},
		{PlatformInstagram, regexp.MustCompile(`(?i)(?:https?://)?(?:www\.)?instagram\.com/(?:p|reels?|tv)/[\w-]+/?(?:[?&#]\S*)?`)},
		{PlatformInstagram, regexp.MustCompile(`(?i)(?:https?://)?(?:www\.)?instagr\.am/(?:p|reels?|tv)/[\w-]+/?(?:[?&#]\S*)?`)},
		{PlatformInstagram, regexp.MustCompile(`(?i)(?:https?://)?(?:www\.)?instagram\.com/share/(?:reel|p)/[\w-]+/?(?:[?&#]\S*)?`)},
		{PlatformInstagram, regexp.MustCompile(`(?i)(?:https?://)?(?:www\.)?instagram\.com/stories/[\w.-]+/\d+/?(?:[?&#]\S*)?`)},
		{PlatformX, regexp.MustCompile(`(?i)(?:https?://)?(?:www\.)?(?:x|twitter)\.com/[\w-]+/status/\d+/?(?:[?&]\S*)?`)},
		{PlatformSpotify, regexp.MustCompile(`(?i)(?:https?://)?(?:open\.)?spotify\.com/(?:album|track)/` + spotifyIDInURL + `(?:[/?#&]\S*)?`)},
		{PlatformSoundCloud, regexp.MustCompile(`(?i)(?:https?://)?(?:www\.)?soundcloud\.com/discover/sets/[^\s?#]+(?:[/?#&]\S*)?`)},
		{PlatformSoundCloud, regexp.MustCompile(`(?i)(?:https?://)?(?:www\.)?soundcloud\.com/[\w.-]+/sets/[^\s?#]+(?:[/?#&]\S*)?`)},
		{PlatformSoundCloud, regexp.MustCompile(`(?i)(?:https?://)?on\.soundcloud\.com/[\w-]+(?:[/?#&]\S*)?`)},
		{PlatformSoundCloud, regexp.MustCompile(`(?i)(?:https?://)?(?:www\.)?soundcloud\.com/[\w.-]+/[\w.-]+(?:[/?#&]\S*)?`)},
		{PlatformPinterest, regexp.MustCompile(`(?i)(?:https?://)?(?:www\.)?pinterest\.com/pin/[\w-]+/?(?:[?&]\S*)?`)},
		{PlatformPinterest, regexp.MustCompile(`(?i)(?:https?://)?pin\.it/[\w-]+/?(?:[?&]\S*)?`)},
		{PlatformPinterest, regexp.MustCompile(`(?i)(?:https?://)?(?:www\.)?pinterest\.com/[\w-]+/[\w-]+/?(?:[?&]\S*)?`)},
	}
)

func IsYouTubeShorts(url string) bool {
	return youtubeShortsRe.MatchString(url)
}

func ExtractURLs(text string) []ParsedLink {
	var results []ParsedLink
	seenSpotify := map[string]struct{}{}

	for _, raw := range urlExtractor.FindAllString(text, -1) {
		url := strings.TrimRight(raw, ".,;:!?)]}")
		matched := false
		for _, p := range patterns {
			if m := p.re.FindString(url); m != "" {
				if p.platform == PlatformSoundCloud && strings.Contains(strings.ToLower(m), "/sets/") {
					continue
				}
				link := ParsedLink{Platform: p.platform, URL: m}
				if p.platform == PlatformX {
					link.XStatusID = ExtractXStatusID(m)
				}
				if p.platform == PlatformSpotify {
					if id, typ := parseSpotifyURL(m); id != "" {
						if _, ok := seenSpotify[id]; ok {
							matched = true
							break
						}
						seenSpotify[id] = struct{}{}
						link.SpotifyID = id
						link.SpotifyTyp = typ
					}
				}
				results = append(results, link)
				matched = true
				break
			}
		}
		if !matched {
			results = append(results, ParsedLink{Platform: PlatformUnknown, URL: url})
		}
	}

	for _, m := range spotifyURIRegex.FindAllStringSubmatch(text, -1) {
		id := m[1]
		if _, ok := seenSpotify[id]; ok {
			continue
		}
		seenSpotify[id] = struct{}{}
		typ := "track"
		if strings.Contains(strings.ToLower(m[0]), "album") {
			typ = "album"
		}
		results = append(results, ParsedLink{
			Platform:   PlatformSpotify,
			URL:        m[0],
			SpotifyID:  id,
			SpotifyTyp: typ,
		})
	}

	return results
}

func ExtractXStatusID(url string) string {
	m := xStatusIDRegex.FindStringSubmatch(url)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func parseSpotifyURL(url string) (id, typ string) {
	re := regexp.MustCompile(`(?i)spotify\.com/(album|track)/([A-Za-z0-9]{22})`)
	m := re.FindStringSubmatch(url)
	if len(m) < 3 {
		return "", ""
	}
	return m[2], strings.ToLower(m[1])
}
