package xphotos

import "testing"

func TestExtractStatusID(t *testing.T) {
	url := "https://x.com/user/status/1234567890?s=20"
	if got := ExtractStatusID(url); got != "1234567890" {
		t.Fatalf("ExtractStatusID() = %q, want 1234567890", got)
	}
}

func TestParseFxTwitter(t *testing.T) {
	body := []byte(`{"tweet":{"text":"hello","author":{"name":"McGriller"},"media":{"photos":[{"url":"https://example.com/a.jpg"}]}}}`)
	tweet, err := parseFxTwitter(body)
	if err != nil {
		t.Fatal(err)
	}
	if tweet.text != "hello" {
		t.Fatalf("text = %q", tweet.text)
	}
	if tweet.author != "McGriller" {
		t.Fatalf("author = %q", tweet.author)
	}
	if len(tweet.photos) != 1 || tweet.photos[0] != "https://example.com/a.jpg" {
		t.Fatalf("photos = %#v", tweet.photos)
	}
}

func TestParseVxTwitter(t *testing.T) {
	body := []byte(`{"text":"hi","author":{"name":"user"},"mediaURLs":["https://example.com/b.png"]}`)
	tweet, err := parseVxTwitter(body)
	if err != nil {
		t.Fatal(err)
	}
	if tweet.text != "hi" || tweet.author != "user" || len(tweet.photos) != 1 {
		t.Fatalf("tweet=%#v", tweet)
	}
}

func TestIsNoVideoError(t *testing.T) {
	if !IsNoVideoError(fmtError("ERROR: no video could be found")) {
		t.Fatal("expected no-video detection")
	}
}

type fmtError string

func (e fmtError) Error() string { return string(e) }
