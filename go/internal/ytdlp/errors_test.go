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
		{platform: "instagram", err: errors.New("operation timed out")},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.platform, func(t *testing.T) {
			t.Parallel()
			if got := UserFacingErrorKey(tt.platform, tt.err); got != "download.timeout" {
				t.Fatalf("got %q, want download.timeout", got)
			}
		})
	}
}

func TestUserFacingErrorKey_instagramAuth(t *testing.T) {
	t.Parallel()
	tests := []string{
		"login required",
		"use --cookies",
		"rate-limit reached",
		"checkpoint required",
		"authentication failed",
	}
	for _, msg := range tests {
		msg := msg
		t.Run(msg, func(t *testing.T) {
			t.Parallel()
			if got := UserFacingErrorKey("instagram", errors.New(msg)); got != "instagram.auth_required" {
				t.Fatalf("got %q, want instagram.auth_required", got)
			}
		})
	}
}

func TestUserFacingErrorKey_instagramGeneric(t *testing.T) {
	t.Parallel()
	if got := UserFacingErrorKey("instagram", errors.New("unable to extract video")); got != "instagram.download_failed" {
		t.Fatalf("got %q, want instagram.download_failed", got)
	}
}

func TestUserFacingErrorKey_otherPlatformsDefaultTimeout(t *testing.T) {
	t.Parallel()
	platforms := []string{"youtube", "tiktok", "x", "pinterest"}
	for _, platform := range platforms {
		platform := platform
		t.Run(platform, func(t *testing.T) {
			t.Parallel()
			if got := UserFacingErrorKey(platform, errors.New("some random failure")); got != "download.timeout" {
				t.Fatalf("got %q, want download.timeout", got)
			}
		})
	}
}

func TestUserFacingErrorKeyInstagramAuth_legacy(t *testing.T) {
	t.Parallel()
	key := UserFacingErrorKey("instagram", fmtError("login required; use --cookies"))
	if key != "instagram.auth_required" {
		t.Fatalf("expected auth key, got %q", key)
	}
}

func fmtError(msg string) error {
	return &simpleError{msg: msg}
}

type simpleError struct{ msg string }

func (e *simpleError) Error() string { return e.msg }

func TestUserFacingErrorKey_instagramReadOnlyCookies(t *testing.T) {
	t.Parallel()
	err := errors.New("OSError: [Errno 30] Read-only file system: '/secrets/instagram_cookies.txt'")
	if got := UserFacingErrorKey("instagram", err); got != "instagram.download_failed" {
		t.Fatalf("got %q, want instagram.download_failed", got)
	}
}

func TestUserFacingErrorKey_instagramEmptyMediaResponse(t *testing.T) {
	t.Parallel()
	err := errors.New("Instagram sent an empty media response. Check if this post is accessible in your browser without being logged-in")
	if got := UserFacingErrorKey("instagram", err); got != "instagram.download_failed" {
		t.Fatalf("got %q, want instagram.download_failed", got)
	}
}
