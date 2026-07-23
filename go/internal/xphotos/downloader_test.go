package xphotos

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// withMirrors points fxTwitterAPI/vxTwitterAPI at test servers and restores
// the originals + selfHostedBaseURL after the test.
func withMirrors(t *testing.T, fx, vx string) {
	t.Helper()
	origFx, origVx, origSelf := fxTwitterAPI, vxTwitterAPI, selfHostedBaseURL
	fxTwitterAPI, vxTwitterAPI = fx, vx
	t.Cleanup(func() {
		fxTwitterAPI, vxTwitterAPI, selfHostedBaseURL = origFx, origVx, origSelf
	})
}

func TestConfigureNormalizesBaseURL(t *testing.T) {
	defer Configure("")
	Configure("https://fx.example.com/")
	if selfHostedBaseURL != "https://fx.example.com" {
		t.Fatalf("selfHostedBaseURL = %q, want trailing slash trimmed", selfHostedBaseURL)
	}
	Configure("  https://fx2.example.com  ")
	if selfHostedBaseURL != "https://fx2.example.com" {
		t.Fatalf("selfHostedBaseURL = %q, want trimmed", selfHostedBaseURL)
	}
}

func TestFetchTweetUnconfiguredUsesOnlyPublicMirrors(t *testing.T) {
	Configure("")
	fx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tweet":{"text":"from fx","author":{"name":"a"},"media":{"photos":[{"url":"https://example.com/a.jpg"}]}}}`))
	}))
	defer fx.Close()
	vx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("vx mirror should not be called when fx mirror succeeds")
	}))
	defer vx.Close()
	withMirrors(t, fx.URL, vx.URL)

	tweet, err := fetchTweet(context.Background(), "123")
	if err != nil {
		t.Fatal(err)
	}
	if tweet.text != "from fx" {
		t.Fatalf("text = %q, want from fx", tweet.text)
	}
}

func TestFetchTweetSelfHostedUsedFirst(t *testing.T) {
	self := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tweet":{"text":"from self-hosted","author":{"name":"a"},"media":{"photos":[{"url":"https://example.com/a.jpg"}]}}}`))
	}))
	defer self.Close()
	fx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("fx mirror should not be called when self-hosted succeeds")
	}))
	defer fx.Close()
	vx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("vx mirror should not be called when self-hosted succeeds")
	}))
	defer vx.Close()
	withMirrors(t, fx.URL, vx.URL)
	Configure(self.URL)
	defer Configure("")

	tweet, err := fetchTweet(context.Background(), "123")
	if err != nil {
		t.Fatal(err)
	}
	if tweet.text != "from self-hosted" {
		t.Fatalf("text = %q, want from self-hosted", tweet.text)
	}
}

func TestFetchTweetSelfHostedFailsFallsBackToPublicMirrors(t *testing.T) {
	self := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer self.Close()
	fx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tweet":{"text":"from fx fallback","author":{"name":"a"},"media":{"photos":[{"url":"https://example.com/a.jpg"}]}}}`))
	}))
	defer fx.Close()
	vx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("vx mirror should not be called when fx mirror succeeds after self-hosted failure")
	}))
	defer vx.Close()
	withMirrors(t, fx.URL, vx.URL)
	Configure(self.URL)
	defer Configure("")

	tweet, err := fetchTweet(context.Background(), "123")
	if err != nil {
		t.Fatal(err)
	}
	if tweet.text != "from fx fallback" {
		t.Fatalf("text = %q, want from fx fallback", tweet.text)
	}
}

func TestFetchTweetSelfHostedEmptyPayloadFallsBack(t *testing.T) {
	self := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tweet":{}}`))
	}))
	defer self.Close()
	fx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tweet":{"text":"from fx after empty","author":{"name":"a"},"media":{"photos":[{"url":"https://example.com/a.jpg"}]}}}`))
	}))
	defer fx.Close()
	vx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("vx mirror should not be called")
	}))
	defer vx.Close()
	withMirrors(t, fx.URL, vx.URL)
	Configure(self.URL)
	defer Configure("")

	tweet, err := fetchTweet(context.Background(), "123")
	if err != nil {
		t.Fatal(err)
	}
	if tweet.text != "from fx after empty" {
		t.Fatalf("text = %q, want from fx after empty", tweet.text)
	}
}

func TestDownloadMediaFallsBackToParentWhenReplyHasNoMedia(t *testing.T) {
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
	withMirrors(t, fx.URL, vx.URL)

	dir := t.TempDir()
	result, items, err := DownloadMedia(context.Background(), "https://x.com/user/status/child", dir, "child", 0)
	if err != nil {
		t.Fatalf("DownloadMedia() error = %v", err)
	}
	if result.Title != "reply text" {
		t.Fatalf("title = %q, want reply text (own text kept, media borrowed from parent)", result.Title)
	}
	if len(items) != 1 {
		t.Fatalf("items = %#v, want 1 downloaded item from parent tweet", items)
	}
}

func TestFetchTweetAllSourcesFail(t *testing.T) {
	self := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer self.Close()
	fx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer fx.Close()
	vx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer vx.Close()
	withMirrors(t, fx.URL, vx.URL)
	Configure(self.URL)
	defer Configure("")

	_, err := fetchTweet(context.Background(), "123")
	if err == nil {
		t.Fatal("expected error when all sources fail")
	}
	// Hard failures (timeouts, HTTP errors) must never be reported as
	// ErrNotFound: that sentinel means "no media", and callers use it to
	// silently label the post as text-only. A transient outage across all
	// mirrors is a real failure and must surface as ErrUnavailable so it
	// gets reported to the user instead of being mislabeled.
	if errors.Is(err, ErrNotFound) {
		t.Fatalf("hard failures must not be reported as ErrNotFound, got %v", err)
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}

func TestFetchTweetEmptyPayloadIsNotFound(t *testing.T) {
	// A source that responds 200 OK with a well-formed but empty tweet
	// (deleted/private/no-media) is genuine "not found" information, unlike
	// a hard failure, and should still map to ErrNotFound.
	self := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tweet":{}}`))
	}))
	defer self.Close()
	fx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer fx.Close()
	vx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer vx.Close()
	withMirrors(t, fx.URL, vx.URL)
	Configure(self.URL)
	defer Configure("")

	_, err := fetchTweet(context.Background(), "123")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

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
	photos := tweet.photoURLs()
	if len(photos) != 1 || photos[0] != "https://example.com/a.jpg" {
		t.Fatalf("photos = %#v", photos)
	}
}

func TestParseVxTwitter(t *testing.T) {
	body := []byte(`{"text":"hi","author":{"name":"user"},"mediaURLs":["https://example.com/b.png"]}`)
	tweet, err := parseVxTwitter(body)
	if err != nil {
		t.Fatal(err)
	}
	if tweet.text != "hi" || tweet.author != "user" || len(tweet.photoURLs()) != 1 {
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
