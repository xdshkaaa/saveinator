package sender

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
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
		{title: "My reel", platform: "tiktok", want: "My reel\n\nvia @saveinator_bot"},
		{title: "", platform: "tiktok", want: "via @saveinator_bot"},
		{title: "", platform: "tiktok", want: "via @saveinator_bot"},
		{title: "Cool video", platform: "youtube", want: "Cool video\n\nvia @saveinator_bot"},
		{title: "", platform: "youtube", want: "via @saveinator_bot"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.platform+"_"+tc.title, func(t *testing.T) {
			t.Parallel()
			if got := buildCaption(tc.title, "en", tc.platform, "saveinator_bot"); got != tc.want {
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

// TestSendFileNilMarkupNotOnWire pins the telego trap: ReplyMarkup params are
// interface-typed, so chaining WithReplyMarkup with a typed-nil
// *InlineKeyboardMarkup serializes reply_markup=null and Telegram answers
// 400 "object expected as reply markup". Nil-markup sends must omit the field
// entirely; a real keyboard must still be delivered.
func TestSendFileNilMarkupNotOnWire(t *testing.T) {
	t.Parallel()

	contentTypes := make([]string, 0, 4)
	bodies := make([][]byte, 0, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, body)
		contentTypes = append(contentTypes, r.Header.Get("Content-Type"))
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer srv.Close()

	bot, err := telego.NewBot("1234567890:"+strings.Repeat("A", 35), telego.WithAPIServer(srv.URL), telego.WithDiscardLogger())
	if err != nil {
		t.Fatal(err)
	}
	s := New(bot)

	path := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(path, bytes.Repeat([]byte{0x00}, 64), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.SendFile(1, path, "t", "en", "tiktok", false); err != nil {
		t.Fatal(err)
	}
	if err := s.SendFileNoFooter(1, path, "t", "en", "tiktok", false); err != nil {
		t.Fatal(err)
	}
	if err := s.SendFileWithMarkup(1, path, "t", "en", "tiktok", false, nil); err != nil {
		t.Fatal(err)
	}
	for i := range bodies {
		if v := multipartField(t, bodies[i], contentTypes[i], "reply_markup"); v != "" {
			t.Fatalf("nil-markup send #%d carried reply_markup=%q, want field absent", i, v)
		}
	}

	keyboard := tu.InlineKeyboard(tu.InlineKeyboardRow(tu.InlineKeyboardButton("b").WithCallbackData("x")))
	if err := s.SendFileWithMarkup(1, path, "t", "en", "tiktok", false, keyboard); err != nil {
		t.Fatal(err)
	}
	if v := multipartField(t, bodies[len(bodies)-1], contentTypes[len(contentTypes)-1], "reply_markup"); v == "" {
		t.Fatal("SendFileWithMarkup with a keyboard sent no reply_markup")
	}
}

// TestSendFileRetriesWithoutCaptionWhenTooLong pins the TikTok caption
// fallback: when Telegram answers 400 "message caption is too long", the send
// must be repeated without the caption, and the retried multipart upload must
// carry the full file again (the reader was consumed by the first request).
func TestSendFileRetriesWithoutCaptionWhenTooLong(t *testing.T) {
	t.Parallel()

	var bodies [][]byte
	var contentTypes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, body)
		contentTypes = append(contentTypes, r.Header.Get("Content-Type"))
		if len(bodies) == 1 {
			w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: message caption is too long"}`))
			return
		}
		w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer srv.Close()

	bot, err := telego.NewBot("1234567890:"+strings.Repeat("A", 35), telego.WithAPIServer(srv.URL), telego.WithDiscardLogger())
	if err != nil {
		t.Fatal(err)
	}
	s := New(bot)

	path := filepath.Join(t.TempDir(), "clip.mp4")
	content := bytes.Repeat([]byte{0xAB}, 64)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	longCaption := strings.Repeat("#hashtag", 200)
	if err := s.SendFile(1, path, longCaption, "en", "tiktok", false); err != nil {
		t.Fatalf("send should succeed on retry, got: %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("expected 2 requests (refused + retry), got %d", len(bodies))
	}

	if v := multipartField(t, bodies[0], contentTypes[0], "caption"); v != longCaption+"\n\nvia @saveinator_bot" {
		t.Fatalf("first request caption = %d chars, want the full long caption", len(v))
	}
	if v := multipartField(t, bodies[1], contentTypes[1], "caption"); v != "" {
		t.Fatalf("retry carried caption %q, want none", v)
	}
	if got := multipartField(t, bodies[1], contentTypes[1], "video"); len(got) != len(content) {
		t.Fatalf("retry uploaded %d video bytes, want full %d", len(got), len(content))
	}
}

// TestSendFileDoesNotRetryOtherErrors pins that only the caption-too-long
// failure triggers the no-caption resend; everything else returns as is.
func TestSendFileDoesNotRetryOtherErrors(t *testing.T) {
	t.Parallel()

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: wrong file identifier/http URL specified"}`))
	}))
	defer srv.Close()

	bot, err := telego.NewBot("1234567890:"+strings.Repeat("A", 35), telego.WithAPIServer(srv.URL), telego.WithDiscardLogger())
	if err != nil {
		t.Fatal(err)
	}
	s := New(bot)

	path := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(path, bytes.Repeat([]byte{0x00}, 64), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.SendFile(1, path, "caption", "en", "tiktok", false); err == nil {
		t.Fatal("expected the non-caption error to be returned")
	}
	if requests != 1 {
		t.Fatalf("expected exactly 1 request, got %d", requests)
	}
}

// multipartField extracts a form value from a captured multipart request body.
func multipartField(t *testing.T, body []byte, contentType, field string) string {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		t.Fatalf("unexpected content type %q: %v", contentType, err)
	}
	mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			return ""
		}
		if err != nil {
			t.Fatal(err)
		}
		if part.FormName() == field {
			value, _ := io.ReadAll(part)
			return string(value)
		}
	}
}
