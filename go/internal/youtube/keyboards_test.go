package youtube

import "testing"

func TestBuildFormat(t *testing.T) {
	format := BuildFormat(1080, "16_9")
	if format == "" || !contains(format, "height<=1080") {
		t.Fatalf("unexpected format: %s", format)
	}
	format = BuildFormat(720, "9_16")
	if !contains(format, "width<=720") {
		t.Fatalf("unexpected format: %s", format)
	}
}

func TestFormatKeyboardLayout(t *testing.T) {
	opts := []Option{{Height: 144}, {Height: 240}, {Height: 360}, {Height: 480}}
	kb := FormatKeyboard("ru", opts, true, true)

	// Three qualities per row, then the leftover, then Mp3, then Trim.
	if len(kb.InlineKeyboard) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(kb.InlineKeyboard))
	}
	if len(kb.InlineKeyboard[0]) != 3 || len(kb.InlineKeyboard[1]) != 1 {
		t.Fatalf("unexpected quality rows: %d, %d", len(kb.InlineKeyboard[0]), len(kb.InlineKeyboard[1]))
	}
	if kb.InlineKeyboard[0][0].CallbackData != "quality:144" {
		t.Fatalf("unexpected callback: %s", kb.InlineKeyboard[0][0].CallbackData)
	}
	if kb.InlineKeyboard[2][0].CallbackData != "yt:mp3" {
		t.Fatalf("unexpected mp3 callback: %s", kb.InlineKeyboard[2][0].CallbackData)
	}
	if kb.InlineKeyboard[3][0].CallbackData != "yt:trim" {
		t.Fatalf("unexpected trim callback: %s", kb.InlineKeyboard[3][0].CallbackData)
	}
}

func TestFormatKeyboardHidesDisabledActions(t *testing.T) {
	kb := FormatKeyboard("ru", []Option{{Height: 720}}, false, false)
	if len(kb.InlineKeyboard) != 1 {
		t.Fatalf("expected only the quality row, got %d rows", len(kb.InlineKeyboard))
	}
}

func TestCallbackDataStaysWithinTelegramLimit(t *testing.T) {
	kb := FormatKeyboard("ru", []Option{{Height: 1080}}, true, true)
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if len(btn.CallbackData) > 64 {
				t.Fatalf("callback data too long: %s", btn.CallbackData)
			}
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || index(s, sub) >= 0)
}

func index(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
