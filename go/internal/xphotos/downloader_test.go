package xphotos

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestDownloadPhotosFallsBackToParentWhenReplyHasNoMedia(t *testing.T) {
	imgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fake-image-bytes"))
	}))
	defer imgServer.Close()

	fx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/child"):
			_, _ = w.Write([]byte(`{"tweet":{"text":"reply text","author":{"name":"a"},"replying_to_status":"parent","media":{}}}`))
		case strings.HasSuffix(r.URL.Path, "/parent"):
			_, _ = fmt.Fprintf(w, `{"tweet":{"text":"parent text","author":{"name":"a"},"media":{"photos":[{"url":%q}]}}}`, imgServer.URL+"/a.jpg")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fx.Close()
	vx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer vx.Close()

	origFx, origVx := fxTwitterAPI, vxTwitterAPI
	fxTwitterAPI, vxTwitterAPI = fx.URL, vx.URL
	defer func() { fxTwitterAPI, vxTwitterAPI = origFx, origVx }()

	dir := t.TempDir()
	result, paths, err := DownloadPhotos(context.Background(), "https://x.com/user/status/child", dir, "child", 0)
	if err != nil {
		t.Fatalf("DownloadPhotos() error = %v", err)
	}
	if result.Title != "reply text" {
		t.Fatalf("title = %q, want reply text (own text kept, media borrowed from parent)", result.Title)
	}
	if len(paths) != 1 {
		t.Fatalf("paths = %#v, want 1 downloaded photo from parent tweet", paths)
	}
}
