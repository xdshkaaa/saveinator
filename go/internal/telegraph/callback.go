package telegraph

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"saveinator/internal/locale"
)

// CallbackPrefix is the inline-keyboard callback prefix for telegraph
// actions; a translate button carries "tg:tr:<userID>:<threadID>".
const CallbackPrefix = "tg:tr:"

// TranslateData is the decoded callback payload of a translate button.
type TranslateData struct {
	UserID   int64
	ThreadID string
}

// TranslateCallbackData encodes the translate button callback data.
func TranslateCallbackData(userID int64, threadID string) string {
	return fmt.Sprintf("tg:tr:%d:%s", userID, threadID)
}

// ParseTranslate decodes a translate button callback; it returns false for
// malformed or stale payloads.
func ParseTranslate(data string) (TranslateData, bool) {
	parts := strings.Split(data, ":")
	if len(parts) != 4 || parts[0] != "tg" || parts[1] != "tr" || parts[3] == "" {
		return TranslateData{}, false
	}
	userID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || userID <= 0 {
		return TranslateData{}, false
	}
	return TranslateData{UserID: userID, ThreadID: parts[3]}, true
}

// TranslateKeyboard returns the inline keyboard of a freshly published
// article: one "translate to Russian" button.
func TranslateKeyboard(lang string, d TranslateData) *telego.InlineKeyboardMarkup {
	if d.UserID <= 0 || d.ThreadID == "" {
		return nil
	}
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(locale.Get("telegraph.btn_translate", lang, nil)).
				WithCallbackData(TranslateCallbackData(d.UserID, d.ThreadID)),
		),
	)
}

// TranslatedKeyboard replaces the translate button with a link to the
// published Russian version.
func TranslatedKeyboard(lang, ruURL string) *telego.InlineKeyboardMarkup {
	if ruURL == "" {
		return nil
	}
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(locale.Get("telegraph.btn_article_ru", lang, nil)).WithURL(ruURL),
		),
	)
}
