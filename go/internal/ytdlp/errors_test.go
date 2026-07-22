package ytdlp

import (
	"errors"
	"testing"
)

func TestUserFacingErrorKey_nil(t *testing.T) {
	t.Parallel()
	if got := UserFacingErrorKey("youtube", nil); got != "" {
		t.Fatalf("expected empty key, got %q", got)
	}
}

func TestUserFacingErrorKey_timeout(t *testing.T) {
	t.Parallel()
	tests := []struct {
		platform string
		err      error
	}{
		{platform: "youtube", err: errors.New("context deadline exceeded")},
		{platform: "tiktok", err: errors.New("download timed out")},
		{platform: "x", err: errors.New("operation timed out")},
	}
	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			t.Parallel()
			if got := UserFacingErrorKey(tt.platform, tt.err); got != "download.timeout" {
				t.Fatalf("got %q, want download.timeout", got)
			}
		})
	}
}

func TestUserFacingErrorKey_unclassifiedIsEmpty(t *testing.T) {
	t.Parallel()
	platforms := []string{"youtube", "tiktok", "x", "pinterest"}
	for _, platform := range platforms {
		t.Run(platform, func(t *testing.T) {
			t.Parallel()
			if got := UserFacingErrorKey(platform, errors.New("some random failure")); got != "" {
				t.Fatalf("got %q, want empty (caller falls back to its own generic message)", got)
			}
		})
	}
}

func TestUserFacingErrorKey_notFound(t *testing.T) {
	t.Parallel()
	tests := []string{
		"ERROR: No video formats found!",
		"no video file found",
		"no media files found",
		"ERROR: [youtube] abc123: Video unavailable",
		"ERROR: Private video. Sign in if you've been granted access to this video",
		"ERROR: This content isn't available",
	}
	for _, msg := range tests {
		t.Run(msg, func(t *testing.T) {
			t.Parallel()
			if got := UserFacingErrorKey("youtube", errors.New(msg)); got != "errors.not_found" {
				t.Fatalf("got %q, want errors.not_found", got)
			}
		})
	}
}

func TestUserFacingErrorKey_rateLimited(t *testing.T) {
	t.Parallel()
	tests := []string{
		"HTTP Error 429: Too Many Requests",
		"ERROR: rate-limit reached",
	}
	for _, msg := range tests {
		t.Run(msg, func(t *testing.T) {
			t.Parallel()
			if got := UserFacingErrorKey("tiktok", errors.New(msg)); got != "errors.rate_limited" {
				t.Fatalf("got %q, want errors.rate_limited", got)
			}
		})
	}
}

func TestUserFacingErrorKey_network(t *testing.T) {
	t.Parallel()
	tests := []string{
		"dial tcp: connection refused",
		"dial tcp: lookup example.com: no such host",
		"read: connection reset by peer",
	}
	for _, msg := range tests {
		t.Run(msg, func(t *testing.T) {
			t.Parallel()
			if got := UserFacingErrorKey("x", errors.New(msg)); got != "errors.network" {
				t.Fatalf("got %q, want errors.network", got)
			}
		})
	}
}

func TestIsNoVideoFormatsError(t *testing.T) {
	t.Parallel()
	if !IsNoVideoFormatsError(errors.New("ERROR: No video formats found!")) {
		t.Fatal("expected true")
	}
	if IsNoVideoFormatsError(errors.New("login required")) {
		t.Fatal("expected false")
	}
}
