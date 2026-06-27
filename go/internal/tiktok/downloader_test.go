package tiktok

import (
	"os"
	"path/filepath"
	"testing"
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
	if args[1] != "/tmp/tiktok_cookies.txt" {
		t.Fatalf("expected writable cookie copy, got %v", args[1])
	}
}
