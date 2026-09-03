package tiktok

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"saveinator/internal/cookies"
)

func TestCookieArgsBrowserFallback(t *testing.T) {
	t.Parallel()
	d := NewDownloader("/missing/tiktok_cookies.txt", "chrome", "", 60, 10, true, 0)
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
	d := NewDownloader(cookieFile, "chrome", "", 60, 10, true, 0)
	args := d.cookieArgs()
	if len(args) != 2 || args[0] != "--cookies" {
		t.Fatalf("expected file cookies, got %v", args)
	}
	if args[1] != cookies.TikTokWritablePath {
		t.Fatalf("expected writable cookie copy, got %v", args[1])
	}
}

func TestRefererArgsIncluded(t *testing.T) {
	t.Parallel()
	d := NewDownloader("", "", TikTokRefererDefault, 60, 10, true, 0)
	args := d.refererArgs()
	if len(args) != 2 || args[0] != "--referer" || args[1] != TikTokRefererDefault {
		t.Fatalf("expected referer args, got %v", args)
	}
}

func TestRefererArgsOmittedWhenEmpty(t *testing.T) {
	t.Parallel()
	d := NewDownloader("", "", "", 60, 10, true, 0)
	args := d.refererArgs()
	if len(args) != 2 || args[0] != "--referer" || args[1] != TikTokRefererDefault {
		t.Fatalf("expected default referer fallback, got %v", args)
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

func TestExtraArgsPlayerClient(t *testing.T) {
	t.Parallel()
	d := NewDownloader("", "", "", 60, 10, true, 0)
	args := d.extraArgs()
	if len(args) != 2 || args[0] != "--extractor-args" || args[1] != TikTokPlayerClient {
		t.Fatalf("expected --extractor-args tiktok:player_client=web, got %v", args)
	}
}

func TestExtraArgsMaxDuration(t *testing.T) {
	t.Parallel()
	d := NewDownloader("", "", "", 60, 10, true, 300)
	args := d.extraArgs()
	if len(args) != 4 || args[0] != "--extractor-args" || args[2] != "--max-duration" || args[3] != "300" {
		t.Fatalf("expected --extractor-args ... --max-duration 300, got %v", args)
	}
}

func TestExtraArgsNoMaxDurationWhenZero(t *testing.T) {
	t.Parallel()
	d := NewDownloader("", "", "", 60, 10, true, 0)
	args := d.extraArgs()
	for _, a := range args {
		if a == "--max-duration" {
			t.Fatal("unexpected --max-duration when maxDuration is 0")
		}
	}
}

const carouselPageHTML = `<!doctype html><html><head>
<script id="__UNIVERSAL_DATA_FOR_REHYDRATION__" type="application/json">{"__DEFAULT_SCOPE__":{"webapp.video-detail":{"itemInfo":{"itemStruct":{"id":"7656787943713033505","desc":"test slideshow","imagePost":{"cover":{},"images":[
{"imageWidth":1080,"imageHeight":1440,"imageURL":{"urlList":["https://p16.tiktokcdn.com/img-a~tplv-photomode-image.jpeg?sig=1","https://p19.tiktokcdn.com/img-a~tplv-photomode-image.jpeg?sig=2"]}},
{"imageWidth":1470,"imageHeight":1070,"imageAdvancedUrls":{"720":{"urlList":["https://p16.tiktokcdn.com/img-b~c5_720x720.jpeg"]},"1080":{"urlList":["https://p16.tiktokcdn.com/img-b~c5_1080x1080.jpeg"]}},"imageURL":{"urlList":["https://p16.tiktokcdn.com/img-b~tplv-photomode-image.jpeg"]}},
{"imageWidth":100,"imageHeight":100,"imageURL":{"urlList":["//p16.tiktokcdn.com/img-c~tplv-photomode-image.jpeg"]}},
{"imageWidth":100,"imageHeight":100}
]}}}}}}</script>
</head><body></body></html>`

func TestExtractCarouselURLsFromHTML(t *testing.T) {
	t.Parallel()
	urls := extractCarouselURLsFromHTML(carouselPageHTML)
	want := []string{
		"https://p16.tiktokcdn.com/img-a~tplv-photomode-image.jpeg?sig=1",
		"https://p16.tiktokcdn.com/img-b~c5_1080x1080.jpeg",
		"https://p16.tiktokcdn.com/img-c~tplv-photomode-image.jpeg",
	}
	if len(urls) != len(want) {
		t.Fatalf("expected %d URLs, got %d: %v", len(want), len(urls), urls)
	}
	for i := range want {
		if urls[i] != want[i] {
			t.Errorf("urls[%d] = %s, want %s", i, urls[i], want[i])
		}
	}
}

func TestExtractCarouselURLsFromHTMLNoUniversalData(t *testing.T) {
	t.Parallel()
	if urls := extractCarouselURLsFromHTML("<html><body>bot challenge</body></html>"); len(urls) != 0 {
		t.Fatalf("expected no URLs from challenge page, got %v", urls)
	}
}

func TestCarouselURLsFromPageDumpsIgnoresChallengePages(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "challenge_www.tiktok.com.dump"), []byte("<html>captcha</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "7656787943713033505_https_-_www.tiktok.com.dump"), []byte(carouselPageHTML), 0o644); err != nil {
		t.Fatal(err)
	}
	urls := carouselURLsFromPageDumps(dir)
	if len(urls) != 3 {
		t.Fatalf("expected 3 URLs from page dumps, got %d: %v", len(urls), urls)
	}
}

func TestCarouselURLsPrefersPageOverJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "page.dump"), []byte(carouselPageHTML), 0o644); err != nil {
		t.Fatal(err)
	}
	info := map[string]any{
		"thumbnails": []any{
			map[string]any{"id": "cover", "url": "https://p16.tiktokcdn.com/img-a~tplv-photomode-image.jpeg"},
			map[string]any{"id": "originCover", "url": "https://p16.tiktokcdn.com/img-a~tplv-photomode-origin.jpeg"},
		},
	}
	d := NewDownloader("", "", "", 60, 10, true, 0)
	urls, fromPage := d.carouselURLs(info, dir)
	if !fromPage {
		t.Fatal("expected fromPage=true when a page dump is available")
	}
	if len(urls) != 3 {
		t.Fatalf("expected 3 slide URLs from page, got %d: %v", len(urls), urls)
	}
}

func TestCarouselURLsFallsBackToJSONWithoutDumps(t *testing.T) {
	t.Parallel()
	info := map[string]any{
		"thumbnails": []any{
			map[string]any{"id": "cover", "url": "https://p16.tiktokcdn.com/img-a~tplv-photomode-image.jpeg"},
			map[string]any{"id": "originCover", "url": "https://p16.tiktokcdn.com/img-a~tplv-photomode-origin.jpeg"},
		},
	}
	d := NewDownloader("", "", "", 60, 10, true, 0)
	urls, fromPage := d.carouselURLs(info, t.TempDir())
	if fromPage {
		t.Fatal("expected fromPage=false without page dumps")
	}
	if len(urls) != 2 {
		t.Fatalf("expected 2 JSON-derived URLs, got %d: %v", len(urls), urls)
	}
}

func TestDownloadHTTPRetriesAfterFailure(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if r.Header.Get("Referer") != TikTokRefererDefault {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte("jpegdata"))
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "image_0000.jpg")
	err := downloadHTTP(context.Background(), &http.Client{Timeout: 5 * time.Second}, srv.URL, path)
	if err != nil {
		t.Fatalf("expected retry to succeed, got %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "jpegdata" {
		t.Fatalf("expected downloaded file content jpegdata, got %q err=%v", data, err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("expected 2 requests (1 failure + 1 retry), got %d", got)
	}
}

func TestDownloadHTTPRejectsNon200(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, "<html>forbidden</html>")
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "image_0001.jpg")
	err := downloadHTTP(context.Background(), &http.Client{Timeout: 5 * time.Second}, srv.URL, path)
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatal("expected no file to be created for non-200 response")
	}
}
