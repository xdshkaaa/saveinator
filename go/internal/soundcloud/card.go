package soundcloud

import (
	"fmt"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/locale"
)

func CardText(release *Release, lang string, downloadEnabled bool) string {
	isSingle := release.ReleaseType == "track" || len(release.Tracks) == 1
	if isSingle && len(release.Tracks) > 0 {
		track := release.Tracks[0]
		lines := []string{
			locale.Get("soundcloud.track_card_title", lang, map[string]string{
				"artist": firstString(track.Artist, release.Artist),
				"title":  firstString(track.Title, release.Title),
			}),
			locale.Get("soundcloud.duration", lang, map[string]string{
				"duration": formatDuration(track.DurationMS),
			}),
		}
		if track.Genre != "" {
			lines = append(lines, locale.Get("soundcloud.genre", lang, map[string]string{"genre": track.Genre}))
		}
		if track.YouTubeFallback {
			lines = append(lines, locale.Get("soundcloud.youtube_fallback_note", lang, nil))
		}
		if !downloadEnabled {
			lines = append(lines, "", locale.Get("soundcloud.no_download", lang, nil))
		}
		return joinLines(lines)
	}

	lines := []string{
		locale.Get("soundcloud.card_title", lang, map[string]string{
			"artist": release.Artist,
			"name":   release.Title,
		}),
		locale.Get("soundcloud.tracks_count", lang, map[string]string{
			"count": fmt.Sprintf("%d", len(release.Tracks)),
		}),
	}
	if releaseUsesYouTubeFallback(release) {
		lines = append(lines, locale.Get("soundcloud.youtube_fallback_note", lang, nil))
	}
	if !downloadEnabled {
		lines = append(lines, "", locale.Get("soundcloud.no_download", lang, nil))
	}
	return joinLines(lines)
}

func OpenKeyboard(release *Release, lang string) *telego.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(locale.Get("soundcloud.btn_open", lang, nil)).WithURL(release.SoundCloudURL),
		),
	)
}

func formatDuration(ms int) string {
	sec := ms / 1000
	if sec < 0 {
		sec = 0
	}
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

func joinLines(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}
