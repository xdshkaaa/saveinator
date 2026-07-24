package ytdlp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppendPlatformCookiesFilePriority(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cookieFile := filepath.Join(dir, "cookies.txt")
	if err := os.WriteFile(cookieFile, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := appendPlatformCookies(nil, Options{
		Platform:                 "tiktok",
		TikTokCookies:            cookieFile,
		TikTokCookiesFromBrowser: "chrome",
	})
	if len(args) != 2 || args[0] != "--cookies" || args[1] != cookieFile {
		t.Fatalf("expected file cookies, got %v", args)
	}
}

func TestAppendPlatformCookiesBrowserFallback(t *testing.T) {
	t.Parallel()
	args := appendPlatformCookies(nil, Options{
		Platform:                 "tiktok",
		TikTokCookies:            "/missing/cookies.txt",
		TikTokCookiesFromBrowser: "chrome",
	})
	if len(args) != 2 || args[0] != "--cookies-from-browser" || args[1] != "chrome" {
		t.Fatalf("expected browser cookies, got %v", args)
	}
}

func TestAppendPlatformCookiesTikTokBrowser(t *testing.T) {
	t.Parallel()
	args := appendPlatformCookies(nil, Options{
		Platform:                 "tiktok",
		TikTokCookiesFromBrowser: "chrome",
	})
	if len(args) != 2 || args[1] != "chrome" {
		t.Fatalf("expected tiktok browser cookies, got %v", args)
	}
}

func TestBuildArgsYouTubePrefersEfficientCodecs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	args := buildArgs("https://youtube.com/watch?v=abc", dir, Options{
		Platform: "youtube",
		FormatID: "best[height<=1080]",
	}, false)

	idx := indexOf(args, "-S")
	if idx == -1 || idx+1 >= len(args) {
		t.Fatalf("expected -S format-sort flag, got %v", args)
	}
	if args[idx+1] != "codec:vp9:av01:h264,+size,+br" {
		t.Fatalf("unexpected format-sort value: %s", args[idx+1])
	}
}

func TestBuildArgsNonYouTubeSkipsCodecSort(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	args := buildArgs("https://tiktok.com/@u/video/1", dir, Options{
		Platform: "tiktok",
		FormatID: "best",
	}, false)

	if indexOf(args, "-S") != -1 {
		t.Fatalf("did not expect -S flag for non-youtube platform, got %v", args)
	}
}

func TestBuildArgsProbeSkipsCodecSort(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	args := buildArgs("https://youtube.com/watch?v=abc", dir, Options{
		Platform: "youtube",
	}, true)

	if indexOf(args, "-S") != -1 {
		t.Fatalf("did not expect -S flag when skipping download, got %v", args)
	}
	if indexOf(args, "--skip-download") == -1 {
		t.Fatalf("expected --skip-download flag, got %v", args)
	}
}

func indexOf(args []string, target string) int {
	for i, a := range args {
		if a == target {
			return i
		}
	}
	return -1
}
