package sender

import (
	"reflect"
	"testing"

	"github.com/mymmrac/telego"
)

func TestTypedNilReplyMarkupInStructFieldIsNonZero(t *testing.T) {
	t.Parallel()
	// telego parseParameters treats struct-field typed-nil ReplyMarkup as non-zero
	// and serializes reply_markup=null → Telegram 400.
	var markup *telego.InlineKeyboardMarkup
	params := &telego.SendVideoParams{
		ChatID:      telego.ChatID{ID: 1},
		ReplyMarkup: markup,
	}
	field := reflect.ValueOf(params).Elem().FieldByName("ReplyMarkup")
	if field.IsZero() {
		t.Fatal("struct field typed-nil ReplyMarkup must be non-zero in reflect; SendFileWithMarkup needs nil guard")
	}
}

func TestEditMessageMarkupNilSafe(t *testing.T) {
	t.Parallel()
	var markup *telego.InlineKeyboardMarkup
	if markup != nil {
		t.Fatal("expected nil markup — EditMessageMarkup must call EditMessage without ReplyMarkup")
	}
}
