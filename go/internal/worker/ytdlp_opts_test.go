package worker

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"saveinator/internal/config"
	"saveinator/internal/cookies"
)

func TestYtdlpOpts_syncsInstagramCookiesFromMount(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mount := filepath.Join(dir, "mount.txt")
	writable := filepath.Join(dir, "writable.txt")
	if err := os.WriteFile(mount, []byte("instagram-cookies"), 0o600); err != nil {
		t.Fatal(err)
	}

	h := &Handler{cfg: &config.Settings{InstagramCookiesPath: mount}}
	opts := h.ytdlpOpts("instagram", "best", time.Minute)

	want := cookies.SyncFromMount(mount, cookies.InstagramWritablePath)
	if want == "" {
		// Writable path is global; verify we at least pass the mount when sync is unavailable.
		if opts.InstagramCookies != mount {
			t.Fatalf("expected mount cookies %q, got %q", mount, opts.InstagramCookies)
		}
		return
	}
	if opts.InstagramCookies != want {
		t.Fatalf("expected synced cookies %q, got %q", want, opts.InstagramCookies)
	}
	_ = writable
}

func TestYtdlpOpts_keepsBrowserCookiesWhenConfigured(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mount := filepath.Join(dir, "mount.txt")
	if err := os.WriteFile(mount, []byte("instagram-cookies"), 0o600); err != nil {
		t.Fatal(err)
	}

	h := &Handler{
		cfg: &config.Settings{
			InstagramCookiesPath:            mount,
			InstagramCookiesFromBrowser:     "chrome",
			TikTokCookiesFromBrowser:        "chrome",
		},
	}
	opts := h.ytdlpOpts("instagram", "best", time.Minute)
	if opts.InstagramCookies != mount {
		t.Fatalf("expected mount path with browser fallback, got %q", opts.InstagramCookies)
	}
	if opts.InstagramCookiesFromBrowser != "chrome" {
		t.Fatalf("expected browser fallback to remain configured")
	}
}
