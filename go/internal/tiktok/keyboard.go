package tiktok

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/locale"
)

func CarouselCallbackData(userID int64, token string) string {
	return fmt.Sprintf("ttk:img:%d:%s", userID, token)
}

func ParseCarouselCallback(data string) (userID int64, token string, ok bool) {
	parts := strings.Split(data, ":")
	if len(parts) != 4 || parts[0] != "ttk" || parts[1] != "img" || parts[3] == "" {
		return 0, "", false
	}
	userID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return 0, "", false
	}
	return userID, parts[3], true
}

func CarouselImagesKeyboard(lang string, userID int64, token string) *telego.InlineKeyboardMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(locale.Get("tiktok.btn_carousel_images", lang, nil)).
				WithCallbackData(CarouselCallbackData(userID, token)),
		),
	)
}
