package yandexmusic

import (
	"fmt"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/locale"
)

// AlbumCallbackPrefix is the inline-button callback prefix that triggers a
// full-album download from a single-track card. Data: ym:alb:<albumID>.
const AlbumCallbackPrefix = "ym:alb:"

func CardText(release *Release, lang string, downloadEnabled bool) string {
	if len(release.Tracks) == 1 {
		track := release.Tracks[0]
		lines := []string{
			locale.Get("yandexmusic.track_card_title", lang, map[string]string{
				"artist": track.Artists,
				"title":  track.Title,
			}),
			locale.Get("yandexmusic.duration", lang, map[string]string{
				"duration": formatDuration(track.DurationMS),
			}),
		}
		if release.AlbumTitle != "" {
			lines = append(lines, locale.Get("yandexmusic.album_name", lang, map[string]string{
				"name": release.AlbumTitle,
			}))
		}
		if !downloadEnabled {
			lines = append(lines, "", locale.Get("yandexmusic.no_download", lang, nil))
		}
		return stringsJoinLines(lines)
	}

	lines := []string{
		locale.Get("yandexmusic.card_title", lang, map[string]string{
			"artist": release.Artists,
			"name":   release.Title,
		}),
		locale.Get("yandexmusic.tracks_count", lang, map[string]string{
			"count": fmt.Sprintf("%d", len(release.Tracks)),
		}),
	}
	if release.ReleaseDate != "" {
		lines = append(lines, locale.Get("yandexmusic.release_year", lang, map[string]string{
			"year": release.ReleaseDate,
		}))
	}
	if !downloadEnabled {
		lines = append(lines, "", locale.Get("yandexmusic.no_download", lang, nil))
	}
	return stringsJoinLines(lines)
}

// OpenKeyboard returns the card keyboard: an "open in Yandex Music" URL
// button plus, for single-track cards with a known multi-track album, an
// album-download callback button.
func OpenKeyboard(release *Release, lang string) *telego.InlineKeyboardMarkup {
	var row []telego.InlineKeyboardButton
	if openURL := release.YandexURL; openURL != "" {
		row = append(row, tu.InlineKeyboardButton(locale.Get("yandexmusic.btn_open", lang, nil)).WithURL(openURL))
	}
	if len(release.Tracks) == 1 && release.AlbumID != "" && release.AlbumTrackCount > 1 {
		row = append(row, tu.InlineKeyboardButton(locale.Get("yandexmusic.btn_download_album", lang, map[string]string{
			"count": fmt.Sprintf("%d", release.AlbumTrackCount),
		})).WithCallbackData(AlbumCallbackPrefix+release.AlbumID))
	}
	if len(row) == 0 {
		return nil
	}
	return tu.InlineKeyboard(tu.InlineKeyboardRow(row...))
}

func formatDuration(ms int) string {
	sec := ms / 1000
	if sec < 0 {
		sec = 0
	}
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
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
