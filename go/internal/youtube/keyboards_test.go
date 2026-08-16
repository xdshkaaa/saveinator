package youtube

import (
	"strings"
	"testing"
)

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

// The chain ends in an uncapped "anything at all" tail on purpose — a video
// with no format at the requested height should still download. What must not
// happen is a capped branch sitting behind an uncapped one, where the cap can
// never be reached and the quality the user picked is silently skipped.
func TestBuildFormatCapsComeBeforeTheUncappedTail(t *testing.T) {
	branches := strings.Split(BuildFormat(720, "16_9"), "/")
	if len(branches) < 2 {
		t.Fatalf("expected a fallback chain, got %q", branches)
	}
	if !strings.Contains(branches[0], "height<=720") {
		t.Fatalf("the chain must open capped, got %q", branches[0])
	}
	uncapped := false
	for _, branch := range branches {
		capped := strings.Contains(branch, "height<=720")
		if uncapped && capped {
			t.Fatalf("capped branch %q is unreachable behind an uncapped one: %v", branch, branches)
		}
		uncapped = uncapped || !capped
	}
}

// YouTube stopped publishing progressive renditions above 360p, so a
// self-contained format has to come after the merged one — ahead of it, every
// request quietly resolves to 360p.
func TestBuildFormatPrefersMergedOverProgressive(t *testing.T) {
	format := BuildFormat(1080, "16_9")
	merged := strings.Index(format, "bestvideo[height<=1080]")
	progressive := strings.Index(format, "best[height<=1080]")
	if merged == -1 || progressive == -1 {
		t.Fatalf("expected both a merged and a progressive branch: %s", format)
	}
	if merged > progressive {
		t.Fatalf("progressive branch precedes the merged one: %s", format)
	}
}

func TestFormatSelector(t *testing.T) {
	selector := FormatSelector("137+140", 1080, "16_9")
	if !strings.HasPrefix(selector, "137+140/") {
		t.Fatalf("probed id must lead so the download matches the advertised size: %s", selector)
	}
	if !strings.Contains(selector, "height<=1080") {
		t.Fatalf("fallback chain missing: %s", selector)
	}
	if got := FormatSelector("  ", 720, "9_16"); got != BuildFormat(720, "9_16") {
		t.Fatalf("empty id should degrade to the generic selector, got %s", got)
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
