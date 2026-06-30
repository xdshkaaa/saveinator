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
		tt := tt
		t.Run(tt.platform, func(t *testing.T) {
			t.Parallel()
			if got := UserFacingErrorKey(tt.platform, tt.err); got != "download.timeout" {
				t.Fatalf("got %q, want download.timeout", got)
			}
		})
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

func TestIsNoVideoFormatsError(t *testing.T) {
	t.Parallel()
	if !IsNoVideoFormatsError(errors.New("ERROR: No video formats found!")) {
		t.Fatal("expected true")
	}
	if IsNoVideoFormatsError(errors.New("login required")) {
		t.Fatal("expected false")
	}
}
