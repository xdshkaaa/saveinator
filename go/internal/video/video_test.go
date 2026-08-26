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

func TestCompressToFitMissingFileReturnsSourceUnchanged(t *testing.T) {
	t.Parallel()
	path := "/tmp/does-not-exist-fit-test.mp4"
	out, ok := CompressToFit(context.Background(), path, 10*1024*1024)
	if ok || out != path {
		t.Fatalf("expected untouched source, got %q ok=%v", out, ok)
	}
}

func TestFitHeight(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		src    int
		budget int
		want   int
	}{
		// Comfortable budget keeps the source resolution.
		{"keeps source when budget fits", 1080, 4000, 1080},
		{"keeps small source even on tight budget", 360, 200, 360},
		// Steps down to the tallest height the budget sustains.
		{"steps 1080 to 720", 1080, 2000, 720},
		{"steps 1080 to 480", 1080, 700, 480},
		{"steps 720 to 360", 720, 500, 360},
		{"floor at 240", 1080, 130, 240},
	}
	for _, tc := range cases {
		if got := fitHeight(tc.src, tc.budget); got != tc.want {
			t.Errorf("%s: fitHeight(%d, %d) = %d, want %d", tc.name, tc.src, tc.budget, got, tc.want)
		}
	}
}

func TestCompressToFitShrinksUnderCapKeepingFilename(t *testing.T) {
	requireFFmpeg(t)
	t.Parallel()
	dir := t.TempDir()
	source := filepath.Join(dir, "BigVideo_dQw4w9WgXcQ.mp4")
	generateHighBitrateSample(t, source)

	before, err := os.Stat(source)
	if err != nil {
		t.Fatalf("stat source: %v", err)
	}

	out, ok := CompressToFit(context.Background(), source, before.Size()/2)
	if !ok {
		t.Fatal("expected compression to fit under the cap")
	}
	if out != source {
		t.Fatalf("expected output to keep original path %q, got %q", source, out)
	}

	after, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat output: %v", err)
	}
	if after.Size() > before.Size()/2 {
		t.Fatalf("expected output under cap %d, got %d", before.Size()/2, after.Size())
	}

	if leftover, _ := filepath.Glob(filepath.Join(dir, "*_fittmp*")); len(leftover) != 0 {
		t.Fatalf("expected no leftover temp files, found %v", leftover)
	}
	if filepath.Base(out) != filepath.Base(source) {
		t.Fatalf("expected filename to be preserved, source=%q out=%q", filepath.Base(source), filepath.Base(out))
	}
}

func TestCompressToFitRefusesImpossibleBudget(t *testing.T) {
	requireFFmpeg(t)
	t.Parallel()
	dir := t.TempDir()
	source := filepath.Join(dir, "HugeVideo_dQw4w9WgXcQ.mp4")
	generateHighBitrateSample(t, source)

	before, statErr := os.Stat(source)
	if statErr != nil {
		t.Fatalf("stat source: %v", statErr)
	}

	// A 16 KB cap for a 3-second clip is far below any watchable bitrate.
	out, ok := CompressToFit(context.Background(), source, 16*1024)
	if ok {
		t.Fatal("expected refusal for impossible budget")
	}
	if out != source {
		t.Fatalf("expected source untouched, got %q", out)
	}

	after, err := os.Stat(source)
	if err != nil {
		t.Fatalf("stat source after refusal: %v", err)
	}
	if after.Size() != before.Size() {
		t.Fatalf("expected source size unchanged, before=%d after=%d", before.Size(), after.Size())
	}
}
