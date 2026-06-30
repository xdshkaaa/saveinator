package handler

import (
	"testing"

	"github.com/mymmrac/telego"
)

func TestFormatPct(t *testing.T) {
	t.Parallel()
	tests := []struct {
		part, total int
		want        string
	}{
		{11, 21, "52%"},
		{0, 0, "—"},
		{5, 10, "50%"},
	}
	for _, tc := range tests {
		if got := formatPct(tc.part, tc.total); got != tc.want {
			t.Fatalf("formatPct(%d, %d) = %q, want %q", tc.part, tc.total, got, tc.want)
		}
	}
}

func TestFormatStickiness(t *testing.T) {
	t.Parallel()
	if got := formatStickiness(2, 11); got != "18%" {
		t.Fatalf("got %q", got)
	}
	if got := formatStickiness(2, 0); got != "—" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatSuccessRate(t *testing.T) {
	t.Parallel()
	if got := formatSuccessRate(168, 12); got != "93% (168/180)" {
		t.Fatalf("got %q", got)
	}
	if got := formatSuccessRate(0, 0); got != "—" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatGrowthDelta(t *testing.T) {
	t.Parallel()
	same := formatGrowthDelta(0, 0, "en")
	if same != " — same as yesterday" {
		t.Fatalf("same = %q", same)
	}
	up := formatGrowthDelta(3, 1, "en")
	if up != " (+2, +200%)" {
		t.Fatalf("up = %q", up)
	}
}

func TestFormatStatsDownloads(t *testing.T) {
	t.Parallel()
	if got := formatStatsDownloads(5, 42, 180); got != "5 · 7d: 42 · 30d: 180" {
		t.Fatalf("got %q", got)
	}
	if got := formatStatsDownloadsRU(5, 42, 180); got != "5 · 7д: 42 · 30д: 180" {
		t.Fatalf("got %q", got)
	}
}

func TestActiveUserID(t *testing.T) {
	t.Parallel()
	uid := int64(42)
	if got := activeUserID(telegoUpdateMessage(uid)); got != uid {
		t.Fatalf("message user = %d", got)
	}
	if got := activeUserID(telegoUpdateCallback(uid)); got != uid {
		t.Fatalf("callback user = %d", got)
	}
}

func telegoUpdateMessage(userID int64) telego.Update {
	return telego.Update{Message: &telego.Message{From: &telego.User{ID: userID}, Chat: telego.Chat{ID: 1}}}
}

func telegoUpdateCallback(userID int64) telego.Update {
	return telego.Update{CallbackQuery: &telego.CallbackQuery{From: telego.User{ID: userID}}}
}
