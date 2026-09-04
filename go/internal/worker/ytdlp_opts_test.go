package worker

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"saveinator/internal/config"
	"saveinator/internal/cookies"
)

func TestYtdlpOpts_syncsTikTokCookiesFromMount(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mount := filepath.Join(dir, "mount.txt")
	if err := os.WriteFile(mount, []byte("tiktok-cookies"), 0o600); err != nil {
		t.Fatal(err)
	}

	h := &Handler{cfg: &config.Settings{TikTokCookiesPath: mount}}
	opts := h.ytdlpOpts("tiktok", "best", time.Minute)

	want := cookies.SyncFromMount(mount, cookies.TikTokWritablePath)
	if want == "" {
		if opts.TikTokCookies != mount {
			t.Fatalf("expected mount cookies %q, got %q", mount, opts.TikTokCookies)
		}
		return
	}
	if opts.TikTokCookies != want {
		t.Fatalf("expected synced cookies %q, got %q", want, opts.TikTokCookies)
	}
}

func TestYtdlpOpts_keepsBrowserCookiesWhenConfigured(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mount := filepath.Join(dir, "mount.txt")
	if err := os.WriteFile(mount, []byte("tiktok-cookies"), 0o600); err != nil {
		t.Fatal(err)
	}

	h := &Handler{
		cfg: &config.Settings{
			TikTokCookiesPath:          mount,
			TikTokCookiesFromBrowser:   "chrome",
		},
	}
	opts := h.ytdlpOpts("tiktok", "best", time.Minute)
	if opts.TikTokCookies != mount {
		t.Fatalf("expected mount path with browser fallback, got %q", opts.TikTokCookies)
	}
	if opts.TikTokCookiesFromBrowser != "chrome" {
		t.Fatalf("expected browser fallback to remain configured")
	}
}

func TestYtdlpOpts_syncsRedditCookiesFromMount(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mount := filepath.Join(dir, "mount.txt")
	if err := os.WriteFile(mount, []byte("reddit-cookies"), 0o600); err != nil {
		t.Fatal(err)
	}

	h := &Handler{cfg: &config.Settings{RedditCookiesPath: mount}}
	opts := h.ytdlpOpts("reddit", "bv*+ba/b", time.Minute)

	want := cookies.SyncFromMount(mount, cookies.RedditWritablePath)
	if want == "" {
		if opts.RedditCookies != mount {
			t.Fatalf("expected mount cookies %q, got %q", mount, opts.RedditCookies)
		}
		return
	}
	if opts.RedditCookies != want {
		t.Fatalf("expected synced cookies %q, got %q", want, opts.RedditCookies)
	}
}
