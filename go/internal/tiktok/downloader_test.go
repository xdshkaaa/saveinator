package tiktok

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"saveinator/internal/cookies"
)

func TestCookieArgsBrowserFallback(t *testing.T) {
	t.Parallel()
	d := NewDownloader("/missing/tiktok_cookies.txt", "chrome", 60, 10, true)
	args := d.cookieArgs()
	if len(args) != 2 || args[0] != "--cookies-from-browser" || args[1] != "chrome" {
		t.Fatalf("expected browser cookies, got %v", args)
	}
}

func TestCookieArgsFilePriority(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cookieFile := filepath.Join(dir, "cookies.txt")
	if err := os.WriteFile(cookieFile, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := NewDownloader(cookieFile, "chrome", 60, 10, true)
	args := d.cookieArgs()
	if len(args) != 2 || args[0] != "--cookies" {
		t.Fatalf("expected file cookies, got %v", args)
	}
	if args[1] != cookies.TikTokWritablePath {
		t.Fatalf("expected writable cookie copy, got %v", args[1])
	}
}

func TestCanonicalPhotoURLRewritesToVideo(t *testing.T) {
	t.Parallel()
	photoURL := "https://www.tiktok.com/@georgefloyd469/photo/7656787943713033505?_r=1&_t=ZS-97ghDdiPeNA"
	got := canonicalVideoURL(photoURL)
	want := "https://www.tiktok.com/@georgefloyd469/video/7656787943713033505"
	if got != want {
		t.Fatalf("photo URL not rewritten to video:\n  got:  %s\n  want: %s", got, want)
	}
}

func TestResolvePageURLConvertsPhotoToVideo(t *testing.T) {
	t.Parallel()
	photoURL := "https://www.tiktok.com/@georgefloyd469/photo/7656787943713033505"
	got := resolvePageURL(photoURL)
	if !strings.Contains(got, "/video/") {
		t.Fatalf("resolvePageURL should convert /photo/ to /video/, got: %s", got)
	}
	if strings.Contains(got, "/photo/") {
		t.Fatalf("resolvePageURL should not retain /photo/, got: %s", got)
	}
}

func TestVideoURLPassesThrough(t *testing.T) {
	t.Parallel()
	videoURL := "https://www.tiktok.com/@user/video/123456789"
	got := canonicalVideoURL(videoURL)
	if got != videoURL {
		t.Fatalf("video URL should pass through unchanged, got: %s", got)
	}
}

func TestShortLinkStripsQueryParams(t *testing.T) {
	t.Parallel()
	shortURL := "https://vm.tiktok.com/ZMk12345/?extra=1"
	got := canonicalVideoURL(shortURL)
	if got != "https://vm.tiktok.com/ZMk12345/" {
		t.Fatalf("short link should strip query params, got: %s", got)
	}
}

func TestPhotoURLQueryParamsStripped(t *testing.T) {
	t.Parallel()
	photoURL := "https://www.tiktok.com/@user/photo/123456789?_r=1&_t=ZS-abc"
	got := canonicalVideoURL(photoURL)
	want := "https://www.tiktok.com/@user/video/123456789"
	if got != want {
		t.Fatalf("photo URL with query params should resolve to clean video URL:\n  got:  %s\n  want: %s", got, want)
	}
}

func TestIsPhotoModeDetectsPhotomodeThumbnail(t *testing.T) {
	t.Parallel()
	info := map[string]any{
		"thumbnails": []any{
			map[string]any{"id": "cover", "url": "https://p16.tiktokcdn.com/tos-useast2a-i-photomode-euttp/image.jpeg"},
			map[string]any{"id": "originCover", "url": "https://p16.tiktokcdn.com/tos-useast2a-i-photomode-euttp/image.jpeg"},
		},
	}
	if !isPhotoMode(info) {
		t.Fatal("expected isPhotoMode to return true for photomode thumbnails")
	}
}

func TestIsPhotoModeFalseForRegularThumbnails(t *testing.T) {
	t.Parallel()
	info := map[string]any{
		"thumbnails": []any{
			map[string]any{"id": "cover", "url": "https://p16.tiktokcdn.com/tos-maliva-avt/image.jpeg"},
		},
	}
	if isPhotoMode(info) {
		t.Fatal("expected isPhotoMode to return false for regular thumbnails")
	}
}

func TestIsPhotoModeFalseWhenNoThumbnails(t *testing.T) {
	t.Parallel()
	info := map[string]any{}
	if isPhotoMode(info) {
		t.Fatal("expected isPhotoMode to return false when no thumbnails")
	}
}

func TestExtractCarouselURLsFromTopLevelThumbnails(t *testing.T) {
	t.Parallel()
	info := map[string]any{
		"thumbnails": []any{
			map[string]any{"id": "cover", "url": "https://example.com/photo1.jpg"},
			map[string]any{"id": "orig", "url": "https://example.com/photo2.webp"},
		},
	}
	urls := extractCarouselURLs(info)
	if len(urls) != 2 {
		t.Fatalf("expected 2 carousel URLs from top-level thumbnails, got %d: %v", len(urls), urls)
	}
}

func TestExtractCarouselURLsFiltersVideoURLs(t *testing.T) {
	t.Parallel()
	info := map[string]any{
		"thumbnails": []any{
			map[string]any{"id": "cover", "url": "https://example.com/video/123.mp4"},
			map[string]any{"id": "orig", "url": "https://example.com/photo.jpg"},
		},
	}
	urls := extractCarouselURLs(info)
	if len(urls) != 1 {
		t.Fatalf("expected 1 carousel URL (video filtered out), got %d: %v", len(urls), urls)
	}
}

func TestPreferVideoDeliveryRejectsAudioOnlyStream(t *testing.T) {
	t.Parallel()
	// photomode post: top-level url is audio-only with vcodec=none
	info := map[string]any{
		"url":     "https://example.com/audio.mp3",
		"vcodec":  "none",
		"acodec":  "mp3",
	}
	if preferVideoDelivery(info) {
		t.Fatal("expected preferVideoDelivery=false for audio-only stream (vcodec=none)")
	}
}

func TestPreferVideoDeliveryAcceptsVideoStream(t *testing.T) {
	t.Parallel()
	info := map[string]any{
		"url":     "https://example.com/video.mp4",
		"vcodec":  "h264",
	}
	if !preferVideoDelivery(info) {
		t.Fatal("expected preferVideoDelivery=true for video stream")
	}
}

func TestPhotoModeEntryTriggersSlideshowDetection(t *testing.T) {
	t.Parallel()
	// Simulates real photomode yt-dlp output: no entries, no is_slideshow,
	// but thumbnails contain photomode URLs.
	info := map[string]any{
		"thumbnails": []any{
			map[string]any{"id": "cover", "url": "https://p16.tiktokcdn.com/tos-useast2a-i-photomode-euttp/c412e909e3d1490b927608a6801582cd~tplv-photomode-image.jpeg"},
		},
	}
	isSlideshow := boolField(info, "is_slideshow") || len(infoEntries(info)) >= 2 || isPhotoMode(info)
	if !isSlideshow {
		t.Fatal("expected photomode post to be detected as slideshow")
	}
	imageURLs := extractCarouselURLs(info)
	if len(imageURLs) == 0 {
		t.Fatal("expected image URLs to be extracted from photomode post")
	}
}
