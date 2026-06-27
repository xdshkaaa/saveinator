package pinterest

import "testing"

func TestParseURLPin(t *testing.T) {
	parsed, err := ParseURL("https://www.pinterest.com/pin/607845280985287827/")
	if err != nil || parsed.URLType != URLTypePin {
		t.Fatalf("unexpected: %+v err=%v", parsed, err)
	}
}

func TestParseURLShort(t *testing.T) {
	parsed, err := ParseURL("https://pin.it/abc123")
	if err != nil || parsed.URLType != URLTypeShort {
		t.Fatalf("unexpected: %+v err=%v", parsed, err)
	}
}

func TestParseURLBoard(t *testing.T) {
	parsed, err := ParseURL("https://www.pinterest.com/user/my-board/")
	if err != nil || parsed.URLType != URLTypeBoard {
		t.Fatalf("unexpected: %+v err=%v", parsed, err)
	}
}

func TestParseURLRuPin(t *testing.T) {
	parsed, err := ParseURL("https://ru.pinterest.com/pin/811985007859293841/")
	if err != nil || parsed.URLType != URLTypePin {
		t.Fatalf("unexpected: %+v err=%v", parsed, err)
	}
}

func TestExtractPinID(t *testing.T) {
	if got := ExtractPinID("https://pinterest.com/pin/123456/"); got != "123456" {
		t.Fatalf("got %q", got)
	}
}
