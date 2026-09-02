package dash

import (
	"strings"
	"testing"
)

func TestTestablePlatform(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		platform string
		url      string
		wantErr  string
	}{
		{
			name:     "youtube watch",
			text:     "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			platform: "youtube",
			url:      "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		},
		{
			name:     "tiktok video",
			text:     "https://www.tiktok.com/@some.user/video/1234567890123456789",
			platform: "tiktok",
			url:      "https://www.tiktok.com/@some.user/video/1234567890123456789",
		},
		{
			name:     "instagram reel with trailing text",
			text:     "check this https://www.instagram.com/reel/ABC123def_/ nice",
			platform: "instagram",
			url:      "https://www.instagram.com/reel/ABC123def_/",
		},
		{
			name:     "x status",
			text:     "https://x.com/someone/status/1234567890123456789",
			platform: "x",
			url:      "https://x.com/someone/status/1234567890123456789",
		},
		{
			name:     "pinterest pin",
			text:     "https://pinterest.com/pin/1234567890/",
			platform: "pinterest",
			url:      "https://pinterest.com/pin/1234567890/",
		},
		{
			name:     "pinterest short link",
			text:     "https://pin.it/abc123",
			platform: "pinterest",
			url:      "https://pin.it/abc123",
		},
		{
			name:     "first link wins",
			text:     "https://pin.it/abc123 and also https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			platform: "pinterest",
			url:      "https://pin.it/abc123",
		},
		{
			name:    "spotify rejected",
			text:    "https://open.spotify.com/track/4PTG3Z6ehGkBF3zIqUnYBi",
			wantErr: "not testable",
		},
		{
			name:    "soundcloud rejected",
			text:    "https://soundcloud.com/artist/sets/setname",
			wantErr: "not testable",
		},
		{
			name:    "unknown platform rejected",
			text:    "https://example.com/some/video",
			wantErr: "not testable",
		},
		{
			name:    "no link at all",
			text:    "just some text",
			wantErr: "no link found",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			platform, url, err := testablePlatform(tc.text)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got err=%v platform=%q url=%q", tc.wantErr, err, platform, url)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if platform != tc.platform {
				t.Errorf("platform = %q, want %q", platform, tc.platform)
			}
			if url != tc.url {
				t.Errorf("url = %q, want %q", url, tc.url)
			}
		})
	}
}
