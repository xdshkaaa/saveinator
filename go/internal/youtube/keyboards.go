package youtube

import (
	"fmt"
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/locale"
)

var (
	ValidQualities = map[int]struct{}{1080: {}, 720: {}, 480: {}}
	ValidRatios    = map[string]struct{}{"16_9": {}, "21_9": {}, "9_16": {}}
)

func QualityKeyboard(lang string) *telego.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("1080p").WithCallbackData("quality:1080"),
			tu.InlineKeyboardButton("720p").WithCallbackData("quality:720"),
			tu.InlineKeyboardButton("480p").WithCallbackData("quality:480"),
		),
	)
}

func RatioKeyboard(lang string) *telego.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton("16:9").WithCallbackData("ratio:16_9"),
			tu.InlineKeyboardButton("21:9").WithCallbackData("ratio:21_9"),
			tu.InlineKeyboardButton("9:16").WithCallbackData("ratio:9_16"),
		),
	)
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
