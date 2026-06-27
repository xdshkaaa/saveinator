package config

import (
	"os"
	"testing"
)

func TestLoadSoundCloudDownloadFromEnvGoDev(t *testing.T) {
	t.Setenv("BOT_TOKEN", "test-token")
	t.Setenv("SOUNDCLOUD_DOWNLOAD_ENABLED", "true")
	t.Setenv("INSTAGRAM_COOKIES_FROM_BROWSER", "chrome")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SoundCloudDownloadEnabled {
		t.Fatal("expected SoundCloudDownloadEnabled=true")
	}
	if cfg.InstagramCookiesPath != "" {
		t.Fatalf("expected empty default instagram cookies path, got %q", cfg.InstagramCookiesPath)
	}
	if cfg.InstagramCookiesFromBrowser != "chrome" {
		t.Fatalf("expected chrome browser cookies, got %q", cfg.InstagramCookiesFromBrowser)
	}
	if cfg.InstagramDownloadTimeoutSeconds != 120 {
		t.Fatalf("expected instagram timeout 120, got %d", cfg.InstagramDownloadTimeoutSeconds)
	}
}

func TestSpotifyAutoEnableWithCredentials(t *testing.T) {
	t.Setenv("BOT_TOKEN", "test-token")
	t.Setenv("SPOTIFY_CLIENT_ID", "id")
	t.Setenv("SPOTIFY_CLIENT_SECRET", "secret")
	os.Unsetenv("SPOTIFY_ENABLED")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SpotifyEnabled {
		t.Fatal("expected SpotifyEnabled=true when credentials present and flag unset")
	}
}

func TestSpotifyExplicitDisable(t *testing.T) {
	t.Setenv("BOT_TOKEN", "test-token")
	t.Setenv("SPOTIFY_CLIENT_ID", "id")
	t.Setenv("SPOTIFY_CLIENT_SECRET", "secret")
	t.Setenv("SPOTIFY_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SpotifyEnabled {
		t.Fatal("expected SpotifyEnabled=false when explicitly disabled")
	}
}
