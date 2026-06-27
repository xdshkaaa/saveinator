package youtube

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/locale"
)

var (
	ValidQualities = map[int]struct{}{1080: {}, 720: {}, 480: {}}
	ValidRatios    = map[string]struct{}{"16_9": {}, "21_9": {}, "9_16": {}}
)

func QualitySetFromStrings(values []string) map[int]struct{} {
	out := make(map[int]struct{}, len(values))
	for _, v := range values {
		q, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			continue
		}
		out[q] = struct{}{}
	}
	if len(out) == 0 {
		return ValidQualities
	}
	return out
}

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

func QualityKeyboard(lang string) *telego.InlineKeyboardMarkup {
	return QualityKeyboardFrom(lang, []string{"1080", "720", "480"})
}

func QualityKeyboardFrom(lang string, allowed []string) *telego.InlineKeyboardMarkup {
	var row []telego.InlineKeyboardButton
	for _, q := range allowed {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		row = append(row, tu.InlineKeyboardButton(q+"p").WithCallbackData("quality:"+q))
	}
	if len(row) == 0 {
		return QualityKeyboard(lang)
	}
	return tu.InlineKeyboard(tu.InlineKeyboardRow(row...))
}

func RatioKeyboard(lang string) *telego.InlineKeyboardMarkup {
	return RatioKeyboardFrom(lang, []string{"16_9", "21_9", "9_16"})
}

func RatioKeyboardFrom(lang string, allowed []string) *telego.InlineKeyboardMarkup {
	var row []telego.InlineKeyboardButton
	for _, ratio := range allowed {
		ratio = strings.TrimSpace(ratio)
		if ratio == "" {
			continue
		}
		row = append(row, tu.InlineKeyboardButton(FormatRatioLabel(ratio)).WithCallbackData("ratio:"+ratio))
	}
	if len(row) == 0 {
		return RatioKeyboard(lang)
	}
	return tu.InlineKeyboard(tu.InlineKeyboardRow(row...))
}

func FormatRatioLabel(ratio string) string {
	return strings.ReplaceAll(ratio, "_", ":")
}

func ProcessingMessage(lang string, quality int, ratio string) string {
	return locale.Get("youtube.processing", lang, map[string]string{
		"quality": fmt.Sprintf("%d", quality),
		"ratio":   FormatRatioLabel(ratio),
	})
}

func BuildFormat(targetHeight int, aspectRatio string) string {
	limitDim := "height"
	if aspectRatio == "9_16" {
		limitDim = "width"
	}
	return fmt.Sprintf(
		"best[%s<=%d][vcodec!=none][acodec!=none]/"+
			"best[%s<=%d][ext=mp4]/"+
			"best[%s<=%d]/"+
			"bestvideo[%s<=%d]+bestaudio/"+
			"bestvideo+bestaudio/best",
		limitDim, targetHeight,
		limitDim, targetHeight,
		limitDim, targetHeight,
		limitDim, targetHeight,
	)
}
