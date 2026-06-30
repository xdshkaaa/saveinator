package cancel

import "testing"

func TestCallbackDataRoundTrip(t *testing.T) {
	t.Parallel()
	data := CallbackData("tiktok", 42, "abc123")
	got, ok := ParseCancel(data)
	if !ok {
		t.Fatal("ParseCancel failed")
	}
	if got.Scenario != "tiktok" || got.UserID != 42 || got.Token != "abc123" {
		t.Fatalf("ParseCancel = %+v", got)
	}
}

func TestParseCancelInvalid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data string
	}{
		{name: "empty", data: ""},
		{name: "wrong prefix", data: "dlq:1"},
		{name: "missing token", data: "dlc:tiktok:1:"},
		{name: "bad user id", data: "dlc:tiktok:x:tok"},
		{name: "too many parts", data: "dlc:a:1:b:c"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, ok := ParseCancel(tt.data); ok {
				t.Fatalf("ParseCancel(%q) = ok, want false", tt.data)
			}
		})
	}
}

func TestQueueCallbackDataRoundTrip(t *testing.T) {
	t.Parallel()
	data := QueueCallbackData(99)
	got, ok := ParseQueue(data)
	if !ok || got != 99 {
		t.Fatalf("ParseQueue(%q) = %d, %v", data, got, ok)
	}
}

func TestParseQueueInvalid(t *testing.T) {
	t.Parallel()
	if _, ok := ParseQueue("dlc:1:2:3"); ok {
		t.Fatal("expected false for cancel prefix")
	}
	if _, ok := ParseQueue("dlq:bad"); ok {
		t.Fatal("expected false for bad user id")
	}
}

func TestKeyboardNilWhenInvalid(t *testing.T) {
	t.Parallel()
	if Keyboard("en", "tiktok", 0, "tok") != nil {
		t.Fatal("expected nil keyboard for userID=0")
	}
	if Keyboard("en", "tiktok", 1, "") != nil {
		t.Fatal("expected nil keyboard for empty token")
	}
	if QueueButton("en", 0) != nil {
		t.Fatal("expected nil queue button for userID=0")
	}
}

func TestScenarioLabel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		scenario string
		want     string
	}{
		{scenario: "spotify", want: "Spotify"},
		{scenario: "soundcloud", want: "SoundCloud"},
		{scenario: "pinterest", want: "Pinterest"},
		{scenario: "tiktok", want: "TikTok"},
		{scenario: "youtube", want: "video"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.scenario, func(t *testing.T) {
			t.Parallel()
			if got := scenarioLabel(tt.scenario); got != tt.want {
				t.Fatalf("scenarioLabel(%q) = %q, want %q", tt.scenario, got, tt.want)
			}
		})
	}
}
