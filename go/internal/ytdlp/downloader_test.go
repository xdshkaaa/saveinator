package ytdlp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppendPlatformCookiesFilePriority(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cookieFile := filepath.Join(dir, "cookies.txt")
	if err := os.WriteFile(cookieFile, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := appendPlatformCookies(nil, Options{
		Platform:                    "instagram",
		InstagramCookies:            cookieFile,
		InstagramCookiesFromBrowser: "chrome",
	})
	if len(args) != 2 || args[0] != "--cookies" || args[1] != cookieFile {
		t.Fatalf("expected file cookies, got %v", args)
	}
}

func TestAppendPlatformCookiesBrowserFallback(t *testing.T) {
	t.Parallel()
	args := appendPlatformCookies(nil, Options{
		Platform:                    "instagram",
		InstagramCookies:            "/missing/cookies.txt",
		InstagramCookiesFromBrowser: "chrome",
	})
	if len(args) != 2 || args[0] != "--cookies-from-browser" || args[1] != "chrome" {
		t.Fatalf("expected browser cookies, got %v", args)
	}
}

func TestAppendPlatformCookiesTikTokBrowser(t *testing.T) {
	t.Parallel()
	args := appendPlatformCookies(nil, Options{
		Platform:                 "tiktok",
		TikTokCookiesFromBrowser: "chrome",
	})
	if len(args) != 2 || args[1] != "chrome" {
		t.Fatalf("expected tiktok browser cookies, got %v", args)
	}
}
