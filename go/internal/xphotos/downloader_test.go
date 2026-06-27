package xphotos

import "testing"

func TestExtractStatusID(t *testing.T) {
	url := "https://x.com/user/status/1234567890?s=20"
	if got := ExtractStatusID(url); got != "1234567890" {
		t.Fatalf("ExtractStatusID() = %q, want 1234567890", got)
	}
}

func TestParseFxTwitter(t *testing.T) {
	body := []byte(`{"tweet":{"text":"hello","media":{"photos":[{"url":"https://example.com/a.jpg"}]}}}`)
	title, urls, err := parseFxTwitter(body)
	if err != nil {
		t.Fatal(err)
	}
	if title != "hello" {
		t.Fatalf("title = %q", title)
	}
	if len(urls) != 1 || urls[0] != "https://example.com/a.jpg" {
		t.Fatalf("urls = %#v", urls)
	}
}

func TestParseVxTwitter(t *testing.T) {
	body := []byte(`{"text":"hi","mediaURLs":["https://example.com/b.png"]}`)
	title, urls, err := parseVxTwitter(body)
	if err != nil {
		t.Fatal(err)
	}
	if title != "hi" || len(urls) != 1 {
		t.Fatalf("title=%q urls=%#v", title, urls)
	}
}

func TestIsNoVideoError(t *testing.T) {
	if !IsNoVideoError(fmtError("ERROR: no video could be found")) {
		t.Fatal("expected no-video detection")
	}
}

type fmtError string

func (e fmtError) Error() string { return string(e) }
