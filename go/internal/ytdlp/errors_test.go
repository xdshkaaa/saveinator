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

func TestUserFacingErrorKey_formatUnavailable(t *testing.T) {
	t.Parallel()
	tests := []string{
		"yt-dlp failed: exit status 1: ERROR: [youtube] abc123: Requested format is not available. Use --list-formats for a list of available formats",
		"WARNING: Only images are available for download. use --list-formats to see them",
		`WARNING: [youtube] abc123: tv_simply client https formats require a GVS PO Token which was not provided.`,
		"WARNING: [youtube] abc123: Some tv client https formats have been skipped as they are DRM protected.",
		// Worded as "Video unavailable", but the video is fine — this must not
		// be reported to the user as removed content.
		"ERROR: [youtube] abc123: Video unavailable. YouTube is requiring a captcha challenge before playback",
	}
	for _, msg := range tests {
		t.Run(msg, func(t *testing.T) {
			t.Parallel()
			if got := UserFacingErrorKey("youtube", errors.New(msg)); got != "errors.format_unavailable" {
				t.Fatalf("got %q, want errors.format_unavailable", got)
			}
		})
	}
}

func TestIsFormatUnavailableError(t *testing.T) {
	t.Parallel()
	if !IsFormatUnavailableError(errors.New("ERROR: Requested format is not available")) {
		t.Fatal("expected true")
	}
	if IsFormatUnavailableError(nil) {
		t.Fatal("expected false for nil")
	}
	// A retry costs a full extraction, so only the format failures qualify.
	for _, msg := range []string{"context deadline exceeded", "connection reset by peer", "ERROR: Private video"} {
		if IsFormatUnavailableError(errors.New(msg)) {
			t.Fatalf("expected false for %q", msg)
		}
	}
}

func TestIsUnexpectedWebpageError(t *testing.T) {
	t.Parallel()
	if !IsUnexpectedWebpageError(errors.New(`ERROR: [TikTok] 7672477995378019616: Unexpected response from webpage request; please report this issue on https://github.com/yt-dlp/yt-dlp/issues`)) {
		t.Fatal("expected true")
	}
	if IsUnexpectedWebpageError(nil) {
		t.Fatal("expected false for nil")
	}
	for _, msg := range []string{"Requested format is not available", "no video formats found"} {
		if IsUnexpectedWebpageError(errors.New(msg)) {
			t.Fatalf("expected false for %q", msg)
		}
	}
}

func TestUserFacingErrorKey_unexpectedWebpage(t *testing.T) {
	t.Parallel()
	// The video still exists — TikTok just served a bot challenge, so this
	// must be the retryable "format unavailable" message, not "removed".
	msg := `yt-dlp failed: exit status 1: ERROR: [TikTok] 7672477995378019616: Unexpected response from webpage request; please report this issue on https://github.com/yt-dlp/yt-dlp/issues`
	if got := UserFacingErrorKey("tiktok", errors.New(msg)); got != "errors.format_unavailable" {
		t.Fatalf("got %q, want errors.format_unavailable", got)
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
