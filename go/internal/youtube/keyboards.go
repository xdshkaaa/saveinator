package youtube

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/locale"
)

var ValidRatios = map[string]struct{}{"16_9": {}, "21_9": {}, "9_16": {}}

const qualitiesPerRow = 3

func RatioSetFromStrings(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			out[v] = struct{}{}
		}
	}
	if len(out) == 0 {
		return ValidRatios
	}
	return out
}

// FormatKeyboard lays out the format card: available qualities three to a row,
// then the audio-only and trim actions.
func FormatKeyboard(lang string, opts []Option, mp3Enabled, trimEnabled bool) *telego.InlineKeyboardMarkup {
	var rows [][]telego.InlineKeyboardButton
	var row []telego.InlineKeyboardButton
	for _, opt := range opts {
		row = append(row, tu.InlineKeyboardButton(
			locale.Get("youtube.btn_quality", lang, map[string]string{"quality": strconv.Itoa(opt.Height)}),
		).WithCallbackData("quality:"+strconv.Itoa(opt.Height)))
		if len(row) == qualitiesPerRow {
			rows = append(rows, tu.InlineKeyboardRow(row...))
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, tu.InlineKeyboardRow(row...))
	}
	if mp3Enabled {
		rows = append(rows, tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(locale.Get("youtube.btn_mp3", lang, nil)).WithCallbackData("yt:mp3"),
		))
	}
	if trimEnabled {
		rows = append(rows, tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(locale.Get("youtube.btn_trim", lang, nil)).WithCallbackData("yt:trim"),
		))
	}
	return tu.InlineKeyboard(rows...)
}

// TrimPromptKeyboard offers a way out of the "send me a range" state.
func TrimPromptKeyboard(lang string) *telego.InlineKeyboardMarkup {
	return tu.InlineKeyboard(tu.InlineKeyboardRow(
		tu.InlineKeyboardButton(locale.Get("youtube.btn_trim_cancel", lang, nil)).WithCallbackData("yt:trimoff"),
	))
}

func FormatRatioLabel(ratio string) string {
	return strings.ReplaceAll(ratio, "_", ":")
}

func ProcessingMessage(lang string, quality int) string {
	return locale.Get("youtube.processing", lang, map[string]string{
		"quality": fmt.Sprintf("%d", quality),
	})
}

func ProcessingAudioMessage(lang string) string {
	return locale.Get("youtube.processing_audio", lang, nil)
}

// FormatSelector turns the format the card advertised into a yt-dlp selector.
//
// The probed id comes first so the download matches the size the user was
// shown, but it is only the first branch: yt-dlp re-extracts the video when it
// downloads, and the format set it sees then does not always match the one the
// probe saw (different player client, a PO-token-gated rendition, or a probe
// answer served from cache). Without a fallback that mismatch fails the whole
// download with "Requested format is not available"; with one it quietly lands
// on the next best rendition at the requested height.
func FormatSelector(formatID string, targetHeight int, aspectRatio string) string {
	fallback := BuildFormat(targetHeight, aspectRatio)
	if strings.TrimSpace(formatID) == "" {
		return fallback
	}
	return formatID + "/" + fallback
}

// BuildFormat is the generic height-capped selector, used on its own when a
// probe yielded no exact format id.
//
// Merged video+audio comes first, and only then a self-contained progressive
// format. The other order looks cheaper — one file, no merge — but YouTube
// stopped publishing progressive renditions above 360p, so it silently answers
// every request with 360p no matter which quality was asked for. Codec
// preference mirrors the probe: h264 + aac merge into a plain mp4 that Telegram
// plays inline.
func BuildFormat(targetHeight int, aspectRatio string) string {
	limitDim := "height"
	if aspectRatio == "9_16" {
		limitDim = "width"
	}
	return fmt.Sprintf(
		"bestvideo[%s<=%d][vcodec^=avc1]+bestaudio[acodec^=mp4a]/"+
			"bestvideo[%s<=%d]+bestaudio/"+
			"best[%s<=%d][ext=mp4]/"+
			"best[%s<=%d]/"+
			"bestvideo+bestaudio/best",
		limitDim, targetHeight,
		limitDim, targetHeight,
		limitDim, targetHeight,
		limitDim, targetHeight,
	)
}
