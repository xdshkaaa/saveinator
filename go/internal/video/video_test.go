package video

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
	_, err := ApplyAspectRatio(context.Background(), "/tmp/video.mp4", "16_9", 2160)
	if err == nil {
		t.Fatal("expected error")
	}
	pe, ok := err.(ProcessingError)
	if !ok || !strings.Contains(pe.Msg, "unsupported quality") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTargetBitrateKbps(t *testing.T) {
	t.Parallel()
	cases := map[int]int{
		2160: 12000,
		1080: 4500,
		1079: 4500,
		720:  2500,
		480:  1200,
		360:  700,
	}
	for height, want := range cases {
		if got := targetBitrateKbps(height); got != want {
			t.Fatalf("targetBitrateKbps(%d) = %d, want %d", height, got, want)
		}
	}
}

func TestCompressIfOversizedMissingFileReturnsSourceUnchanged(t *testing.T) {
	t.Parallel()
	path := "/tmp/does-not-exist-saveinator-test.mp4"
	out, err := CompressIfOversized(context.Background(), path, 600)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != path {
		t.Fatalf("expected unchanged path %q, got %q", path, out)
	}
}

func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not available")
	}
}

// generateHighBitrateSample writes a short, high-entropy (hard to compress)
// clip whose encoded bitrate sits well above the 720p target, so it reliably
// triggers CompressIfOversized's re-encode path.
func generateHighBitrateSample(t *testing.T, path string) {
	t.Helper()
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "nullsrc=size=1280x720:rate=30:duration=3",
		"-filter_complex", "geq=random(1)*255:128:128",
		"-c:v", "libx264", "-b:v", "8000k", "-pix_fmt", "yuv420p", path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to generate sample video: %v: %s", err, out)
	}
}

func TestCompressIfOversizedShrinksHighBitrateVideoKeepingFilename(t *testing.T) {
	requireFFmpeg(t)
	t.Parallel()
	dir := t.TempDir()
	source := filepath.Join(dir, "MyVideo_dQw4w9WgXcQ.mp4")
	generateHighBitrateSample(t, source)

	before, err := os.Stat(source)
	if err != nil {
		t.Fatalf("stat source: %v", err)
	}

	out, err := CompressIfOversized(context.Background(), source, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != source {
		t.Fatalf("expected compressed output to keep original path %q, got %q", source, out)
	}

	after, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat output: %v", err)
	}
	if after.Size() >= before.Size() {
		t.Fatalf("expected compression to shrink file: before=%d after=%d", before.Size(), after.Size())
	}

	if leftover, _ := filepath.Glob(filepath.Join(dir, "*_compressed_tmp*")); len(leftover) != 0 {
		t.Fatalf("expected no leftover temp files, found %v", leftover)
	}
	// Filename must be preserved exactly: downstream title parsing
	// (youtube.DisplayTitle) keys off the filename and doesn't know how to
	// strip a "_compressed" suffix, so gaining one would mangle the title
	// shown to users.
	if filepath.Base(out) != filepath.Base(source) {
		t.Fatalf("expected filename to be preserved, source=%q out=%q", filepath.Base(source), filepath.Base(out))
	}
}

func TestCompressIfOversizedSkipsShortVideos(t *testing.T) {
	requireFFmpeg(t)
	t.Parallel()
	dir := t.TempDir()
	source := filepath.Join(dir, "short_dQw4w9WgXcQ.mp4")
	generateHighBitrateSample(t, source)

	out, err := CompressIfOversized(context.Background(), source, 3600)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != source {
		t.Fatalf("expected short video to be left untouched, got %q", out)
	}
}
