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

func TestBuildCaption(t *testing.T) {
	t.Parallel()
	tests := []struct {
		title    string
		platform string
		want     string
	}{
		{title: "My reel", platform: "instagram", want: "My reel\n\nvia @saveinator_bot"},
		{title: "", platform: "instagram", want: "via @saveinator_bot"},
		{title: "", platform: "tiktok", want: "via @saveinator_bot"},
		{title: "Cool video", platform: "youtube", want: "Cool video\n\nvia @saveinator_bot"},
		{title: "", platform: "youtube", want: "via @saveinator_bot"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.platform+"_"+tc.title, func(t *testing.T) {
			t.Parallel()
			if got := buildCaption(tc.title, "en", tc.platform); got != tc.want {
				t.Fatalf("buildCaption(%q, en, %q) = %q, want %q", tc.title, tc.platform, got, tc.want)
			}
		})
	}
}

func TestEditMessageMarkupNilSafe(t *testing.T) {
	t.Parallel()
	var markup *telego.InlineKeyboardMarkup
	if markup != nil {
		t.Fatal("expected nil markup — EditMessageMarkup must call EditMessage without ReplyMarkup")
	}
}
