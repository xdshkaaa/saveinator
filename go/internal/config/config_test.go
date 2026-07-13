package config

import (
	"os"
	"testing"
)

func TestLoadSoundCloudDownloadFromEnvGoDev(t *testing.T) {
	t.Setenv("BOT_TOKEN", "test-token")
	t.Setenv("INTERNAL_API_TOKEN", "test-internal-token")
	t.Setenv("SOUNDCLOUD_DOWNLOAD_ENABLED", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SoundCloudDownloadEnabled {
		t.Fatal("expected SoundCloudDownloadEnabled=true")
	}
}

func TestSpotifyAutoEnableWithCredentials(t *testing.T) {
	t.Setenv("BOT_TOKEN", "test-token")
	t.Setenv("INTERNAL_API_TOKEN", "test-internal-token")
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
	t.Setenv("INTERNAL_API_TOKEN", "test-internal-token")
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

func TestLoad_requiresInternalAPITokenWhenDownloadAPIEnabled(t *testing.T) {
	t.Setenv("BOT_TOKEN", "test-token")
	os.Unsetenv("INTERNAL_API_TOKEN")
	t.Setenv("DOWNLOAD_API_ENABLED", "true")

	if _, err := Load(); err == nil {
		t.Fatal("expected error when INTERNAL_API_TOKEN is unset and DOWNLOAD_API_ENABLED=true")
	}
}

func TestLoad_downloadAPIDisabledSkipsTokenRequirement(t *testing.T) {
	t.Setenv("BOT_TOKEN", "test-token")
	os.Unsetenv("INTERNAL_API_TOKEN")
	t.Setenv("DOWNLOAD_API_ENABLED", "false")

	if _, err := Load(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
