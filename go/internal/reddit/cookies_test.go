package reddit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCookieHeader(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "cookies.txt")
	content := "# Netscape HTTP Cookie File\n" +
		"#HttpOnly_.reddit.com\tTRUE\t/\tTRUE\t9999999999\ttoken_v2\tabc\n" +
		".reddit.com\tTRUE\t/\tTRUE\t9999999999\tcsv\t2\n" +
		".reddit.com\tTRUE\t/\tTRUE\t1000\texpired\tgone\n" +
		".example.com\tTRUE\t/\tTRUE\t9999999999\tforeign\tnope\n" +
		".reddit.com\tTRUE\t/\tTRUE\t1823061372\tedgebucket\tXYZ\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	got := LoadCookieHeader(path)
	want := "token_v2=abc; csv=2; edgebucket=XYZ"
	if got != want {
		t.Fatalf("header = %q, want %q", got, want)
	}
}

func TestLoadCookieHeaderMissingFile(t *testing.T) {
	t.Parallel()
	if got := LoadCookieHeader("/missing/cookies.txt"); got != "" {
		t.Fatalf("expected empty header, got %q", got)
	}
}

func TestThreadSendsCookieHeader(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "cookies.txt")
	if err := os.WriteFile(path, []byte(
		".reddit.com\tTRUE\t/\tTRUE\t9999999999\ttoken_v2\tabc\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}

	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		_, _ = w.Write([]byte(threadJSONSingleton))
	}))
	defer srv.Close()

	c := NewClient(5, path)
	c.apiBase = srv.URL
	if _, err := c.Thread(context.Background(), "1abcde", 0); err != nil {
		t.Fatalf("Thread: %v", err)
	}
	if gotCookie != "token_v2=abc" {
		t.Fatalf("cookie header = %q, want %q", gotCookie, "token_v2=abc")
	}
}

func TestThreadNotFoundOn403(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := NewClient(5, "")
	c.apiBase = srv.URL
	if _, err := c.Thread(context.Background(), "1abcde", 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// threadJSONSingleton is a minimal single-post thread payload shared by
// header tests that only care about request attributes.
const threadJSONSingleton = `[
  {"data": {"children": [{"kind": "t3", "data": {"id": "1abcde", "title": "t", "permalink": "/r/x/comments/1abcde/t/"}}]}},
  {"data": {"children": []}}
]`
