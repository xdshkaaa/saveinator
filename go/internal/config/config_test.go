package config

import (
	"os"
	"testing"
)

func TestLoadSoundCloudDownloadFromEnvGoDev(t *testing.T) {
	t.Setenv("BOT_TOKEN", "test-token")
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
