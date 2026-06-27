package spotify

import (
	"fmt"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/locale"
)

func CardText(release *Release, lang string, downloadEnabled bool) string {
	isSingle := release.AlbumType == "track" || len(release.Tracks) == 1
	if isSingle && len(release.Tracks) > 0 {
		track := release.Tracks[0]
		lines := []string{
			locale.Get("spotify.track_card_title", lang, map[string]string{
				"artist": track.Artists,
				"title":  track.Title,
			}),
			locale.Get("spotify.duration", lang, map[string]string{
				"duration": formatDuration(track.DurationMS),
			}),
		}
		if !downloadEnabled {
			lines = append(lines, "", locale.Get("spotify.no_download", lang, nil))
		}
		return stringsJoinLines(lines)
	}

	lines := []string{
		locale.Get("spotify.card_title", lang, map[string]string{
			"artist": release.Artists,
			"name":   release.Title,
		}),
		locale.Get("spotify.album_type", lang, map[string]string{
			"type": albumTypeLabel(release.AlbumType, lang),
		}),
		locale.Get("spotify.tracks_count", lang, map[string]string{
			"count": fmt.Sprintf("%d", len(release.Tracks)),
		}),
		locale.Get("spotify.release_date", lang, map[string]string{
			"date": release.ReleaseDate,
		}),
	}
	if !downloadEnabled {
		lines = append(lines, "", locale.Get("spotify.no_download", lang, nil))
	}
	return stringsJoinLines(lines)
}

func OpenKeyboard(release *Release, lang string) *telego.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(locale.Get("spotify.btn_open", lang, nil)).WithURL(release.SpotifyURL),
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

func albumTypeLabel(typ, lang string) string {
	key := "spotify.type_" + typ
	label := locale.Get(key, lang, nil)
	if label == key {
		return typ
	}
	return label
}

func stringsJoinLines(lines []string) string {
	out := ""
	for i, line := range lines {
		if i > 0 {
			out += "\n"
		}
		out += line
	}
	return out
}
