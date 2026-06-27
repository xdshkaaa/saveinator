package worker

import (
	"testing"

	"saveinator/internal/config"
)

func TestRefreshTikTokCookies_disabled(t *testing.T) {
	t.Parallel()
	cfg := &config.Settings{TikTokCookiesRefreshEnabled: false}
	// refreshTikTokCookies returns early without panicking
	refreshTikTokCookies(t.Context(), cfg)
}

func TestRefreshInstagramCookies_disabled(t *testing.T) {
	t.Parallel()
	cfg := &config.Settings{InstagramCookiesRefreshEnabled: false}
	refreshInstagramCookies(t.Context(), cfg)
}

func TestRefreshTikTokCookies_missingConfig(t *testing.T) {
	t.Parallel()
	cfg := &config.Settings{
		TikTokCookiesRefreshEnabled: true,
		TikTokCookiesRefreshURL:     "https://example.com",
	}
	refreshTikTokCookies(t.Context(), cfg)
}

func TestRefreshInstagramCookies_missingConfig(t *testing.T) {
	t.Parallel()
	cfg := &config.Settings{
		InstagramCookiesRefreshEnabled: true,
		InstagramCookiesRefreshURL:     "https://example.com",
	}
	refreshInstagramCookies(t.Context(), cfg)
}
