package worker

import (
	"context"
	"testing"

	"saveinator/internal/config"
)

func TestRefreshTikTokCookies_disabled(t *testing.T) {
	t.Parallel()
	cfg := &config.Settings{TikTokCookiesRefreshEnabled: false}
	refreshTikTokCookies(context.Background(), cfg)
}

func TestRefreshTikTokCookies_missingConfig(t *testing.T) {
	t.Parallel()
	cfg := &config.Settings{
		TikTokCookiesRefreshEnabled: true,
		TikTokCookiesRefreshURL:     "https://example.com",
	}
	refreshTikTokCookies(context.Background(), cfg)
}
