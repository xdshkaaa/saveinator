package cancel

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/locale"
)

type Data struct {
	Scenario string
	UserID   int64
	Token    string
}

func CallbackData(scenario string, userID int64, token string) string {
	return fmt.Sprintf("dlc:%s:%d:%s", scenario, userID, token)
}

func QueueCallbackData(userID int64) string {
	return fmt.Sprintf("dlq:%d", userID)
}

func ParseCancel(data string) (*Data, bool) {
	parts := strings.Split(data, ":")
	if len(parts) != 4 || parts[0] != "dlc" || parts[3] == "" {
		return nil, false
	}
	userID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil, false
	}
	return &Data{Scenario: parts[1], UserID: userID, Token: parts[3]}, true
}

func ParseQueue(data string) (int64, bool) {
	parts := strings.Split(data, ":")
	if len(parts) != 2 || parts[0] != "dlq" {
		return 0, false
	}
	userID, err := strconv.ParseInt(parts[1], 10, 64)
	return userID, err == nil
}

func Keyboard(lang, scenario string, userID int64, token string) *telego.InlineKeyboardMarkup {
	if userID == 0 || token == "" {
		return nil
	}
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(locale.Get("download.cancel", lang, nil)).
				WithCallbackData(CallbackData(scenario, userID, token)),
		),
	)
}

func QueueButton(lang string, userID int64) *telego.InlineKeyboardMarkup {
	if userID == 0 {
		return nil
	}
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(locale.Get("download.queue_button", lang, nil)).
				WithCallbackData(QueueCallbackData(userID)),
		),
	)
}

func ActiveQueueKeyboard(lang, scenario string, userID int64, token string) *telego.InlineKeyboardMarkup {
	label := scenarioLabel(scenario)
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(locale.Get("download.queue_remove", lang, map[string]string{"item": label})).
				WithCallbackData(CallbackData(scenario, userID, token)),
		),
	)
}

func scenarioLabel(scenario string) string {
	switch scenario {
	case "spotify":
		return "Spotify"
	case "soundcloud":
		return "SoundCloud"
	case "pinterest":
		return "Pinterest"
	case "tiktok":
		return "TikTok"
	default:
		return "video"
	}
}
