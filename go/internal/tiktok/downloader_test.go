package tiktok

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"saveinator/internal/cookies"
)

func TestCookieArgsBrowserFallback(t *testing.T) {
	t.Parallel()
	d := NewDownloader("/missing/tiktok_cookies.txt", "chrome", 60, 10, true)
	args := d.cookieArgs()
	if len(args) != 2 || args[0] != "--cookies-from-browser" || args[1] != "chrome" {
		t.Fatalf("expected browser cookies, got %v", args)
	}
}

func TestCookieArgsFilePriority(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cookieFile := filepath.Join(dir, "cookies.txt")
	if err := os.WriteFile(cookieFile, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := NewDownloader(cookieFile, "chrome", 60, 10, true)
	args := d.cookieArgs()
	if len(args) != 2 || args[0] != "--cookies" {
		t.Fatalf("expected file cookies, got %v", args)
	}
	if args[1] != cookies.TikTokWritablePath {
		t.Fatalf("expected writable cookie copy, got %v", args[1])
	}
}

func TestCanonicalPhotoURLRewritesToVideo(t *testing.T) {
	t.Parallel()
	photoURL := "https://www.tiktok.com/@georgefloyd469/photo/7656787943713033505?_r=1&_t=ZS-97ghDdiPeNA"
	got := canonicalVideoURL(photoURL)
	want := "https://www.tiktok.com/@georgefloyd469/video/7656787943713033505"
	if got != want {
		t.Fatalf("photo URL not rewritten to video:\n  got:  %s\n  want: %s", got, want)
	}
}

func TestResolvePageURLConvertsPhotoToVideo(t *testing.T) {
	t.Parallel()
	photoURL := "https://www.tiktok.com/@georgefloyd469/photo/7656787943713033505"
	got := resolvePageURL(photoURL)
	if !strings.Contains(got, "/video/") {
		t.Fatalf("resolvePageURL should convert /photo/ to /video/, got: %s", got)
	}
	if strings.Contains(got, "/photo/") {
		t.Fatalf("resolvePageURL should not retain /photo/, got: %s", got)
	}
}

func TestVideoURLPassesThrough(t *testing.T) {
	t.Parallel()
	videoURL := "https://www.tiktok.com/@user/video/123456789"
	got := canonicalVideoURL(videoURL)
	if got != videoURL {
		t.Fatalf("video URL should pass through unchanged, got: %s", got)
	}
}

func TestShortLinkStripsQueryParams(t *testing.T) {
	t.Parallel()
	shortURL := "https://vm.tiktok.com/ZMk12345/?extra=1"
	got := canonicalVideoURL(shortURL)
	if got != "https://vm.tiktok.com/ZMk12345/" {
		t.Fatalf("short link should strip query params, got: %s", got)
	}
}

func TestPhotoURLQueryParamsStripped(t *testing.T) {
	t.Parallel()
	photoURL := "https://www.tiktok.com/@user/photo/123456789?_r=1&_t=ZS-abc"
	got := canonicalVideoURL(photoURL)
	want := "https://www.tiktok.com/@user/video/123456789"
	if got != want {
		t.Fatalf("photo URL with query params should resolve to clean video URL:\n  got:  %s\n  want: %s", got, want)
	}
}
