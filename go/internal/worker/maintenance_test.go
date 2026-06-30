package worker

import (
	"context"
	"testing"

	"saveinator/internal/config"
)

func TestRefreshTikTokCookies_disabled(t *testing.T) {
	t.Parallel()
	cfg := &config.Settings{TikTokCookiesRefreshEnabled: false}
	// refreshTikTokCookies returns early without panicking
	refreshTikTokCookies(context.Background(), cfg)
}

func TestRefreshInstagramCookies_disabled(t *testing.T) {
	t.Parallel()
	cfg := &config.Settings{InstagramCookiesRefreshEnabled: false}
	refreshInstagramCookies(context.Background(), cfg)
}

func TestRefreshTikTokCookies_missingConfig(t *testing.T) {
	t.Parallel()
	cfg := &config.Settings{
		TikTokCookiesRefreshEnabled: true,
		TikTokCookiesRefreshURL:     "https://example.com",
	}
	refreshTikTokCookies(context.Background(), cfg)
}

func TestRefreshInstagramCookies_missingConfig(t *testing.T) {
	t.Parallel()
	cfg := &config.Settings{
		InstagramCookiesRefreshEnabled: true,
		InstagramCookiesRefreshURL:     "https://example.com",
	}
	refreshInstagramCookies(context.Background(), cfg)
}
