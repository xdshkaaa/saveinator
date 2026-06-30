package video

import (
	"context"
	"strings"
	"testing"
)

func TestProcessingError(t *testing.T) {
	t.Parallel()
	err := ProcessingError{Msg: "ffmpeg failed"}
	if err.Error() != "ffmpeg failed" {
		t.Fatalf("Error() = %q", err.Error())
	}
}

func TestApplyAspectRatioUnsupportedRatio(t *testing.T) {
	t.Parallel()
	_, err := ApplyAspectRatio(context.Background(), "/tmp/video.mp4", "4_3", 1080)
	if err == nil {
		t.Fatal("expected error")
	}
	pe, ok := err.(ProcessingError)
	if !ok || !strings.Contains(pe.Msg, "unsupported aspect ratio") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyAspectRatioUnsupportedQuality(t *testing.T) {
	t.Parallel()
	_, err := ApplyAspectRatio(context.Background(), "/tmp/video.mp4", "16_9", 360)
	if err == nil {
		t.Fatal("expected error")
	}
	pe, ok := err.(ProcessingError)
	if !ok || !strings.Contains(pe.Msg, "unsupported quality") {
		t.Fatalf("unexpected error: %v", err)
	}
}
