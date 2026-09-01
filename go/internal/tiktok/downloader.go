package tiktok

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"saveinator/internal/cookies"
	"saveinator/internal/video"
)

type PostType string

const (
	PostTypeVideo      PostType = "video"
	PostTypeCarousel   PostType = "carousel"
	PostTypeAudioOnly  PostType = "audio_only"
	PostTypeUnknown    PostType = "unknown"
)

type Result struct {
	PostType                 PostType
	Title                    string
	Author                   string
	Images                   []string
	VideoPath                string
	AudioPath                string
	CarouselImagesAvailable  bool
	CarouselImageCount       int
}

var (
	pageItemRE = regexp.MustCompile(`(?i)https?://(?:www\.)?tiktok\.com/@([\w.-]*)/(photo|video)/(\d+)`)
	itemIDRE   = regexp.MustCompile(`(?i)/(?:photo|video)/(\d+)`)
)

// TikTokRefererDefault is sent as the HTTP Referer header on every TikTok
// request. TikTok's CDN serves a bot-challenge page (yt-dlp then fails with
// "Unexpected response from webpage request") when the header is missing.
const TikTokRefererDefault = "https://www.tiktok.com/"

// TikTokPlayerClient forces the TikTok extractor to use the web player
// client. Without it TikTok's CDN serves a bot-challenge page and
// yt-dlp fails with "Unexpected response from webpage request".
const TikTokPlayerClient = "tiktok:player_client=web"

type Downloader struct {
	cookiesPath        string
	cookiesFromBrowser string
	referer            string
	timeout            time.Duration
	maxImages          int
	audioEnabled       bool
	maxDuration        int
}

func NewDownloader(cookiesPath, cookiesFromBrowser, referer string, timeoutSeconds, maxImages int, audioEnabled bool, maxDuration int) *Downloader {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &Downloader{
		cookiesPath:        writableCookies(cookiesPath),
		cookiesFromBrowser: strings.TrimSpace(cookiesFromBrowser),
		referer:            strings.TrimSpace(referer),
		timeout:            timeout,
		maxImages:          maxImages,
		audioEnabled:       audioEnabled,
		maxDuration:        maxDuration,
	}
}

func (d *Downloader) Download(ctx context.Context, url, outputDir string) (*Result, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, err
	}

	info, resolved, err := d.extractInfo(ctx, url)
	if err != nil {
		return nil, err
	}

	title, author := metadataFromInfo(info)
	result := &Result{Title: title, Author: author}

	imageURLs := extractCarouselURLs(info)
	isSlideshow := boolField(info, "is_slideshow") || len(infoEntries(info)) >= 2 || isPhotoMode(info)
	hasImages := len(imageURLs) >= 2 || (isPhotoMode(info) && len(imageURLs) >= 1)

	if isSlideshow && hasImages && !preferVideoDelivery(info) {
		images := d.downloadImages(ctx, imageURLs, outputDir)
		if len(images) > 0 {
			result.PostType = PostTypeCarousel
			result.Images = images
			if d.audioEnabled {
				if audio, err := d.downloadAudio(ctx, resolved, outputDir); err == nil {
					result.AudioPath = audio
				}
			}
			return result, nil
		}
	}

	// Prefer h264: TikTok's hevc/bytevc1 renditions are mislabeled by yt-dlp
	// as having audio when they're actually video-only, and hevc also risks
	// Telegram re-transcoding it server-side for playback compatibility,
	// which has been observed to mangle the aspect ratio. h264 renditions
	// are reliably muxed with real audio and need no server-side transcode.
	if err := d.runYTDLP(ctx, resolved, outputDir, "best[vcodec=h264]/best[acodec!=none]/download/best"); err != nil {
		return nil, err
	}

	images, videoPath, audio := findMediaFiles(outputDir)
	if videoPath != "" {
		if fixed, err := d.ensureAudio(ctx, videoPath, resolved, outputDir); err == nil {
			videoPath = fixed
		}
		if fixed, err := video.NormalizeSAR(ctx, videoPath); err == nil {
			videoPath = fixed
		}
	}
	result.Images = images
	result.VideoPath = videoPath
	result.AudioPath = audio
	result.PostType = detectPostType(images, videoPath, audio)

	if len(imageURLs) >= 2 {
		result.CarouselImagesAvailable = true
		result.CarouselImageCount = len(imageURLs)
	}
	return result, nil
}

// DownloadCarouselImages downloads only slideshow images from a TikTok post.
func (d *Downloader) DownloadCarouselImages(ctx context.Context, url, outputDir string) (*Result, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, err
	}

	info, _, err := d.extractInfo(ctx, url)
	if err != nil {
		return nil, err
	}

	title, author := metadataFromInfo(info)
	imageURLs := extractCarouselURLs(info)
	images := d.downloadImages(ctx, imageURLs, outputDir)

	result := &Result{
		Title:              title,
		Author:             author,
		Images:             images,
		CarouselImageCount: len(imageURLs),
	}
	if len(images) > 0 {
		result.PostType = PostTypeCarousel
	}
	return result, nil
}

func (d *Downloader) extractInfo(ctx context.Context, url string) (map[string]any, string, error) {
	resolved := resolvePageURL(url)
	args := []string{"--dump-single-json", "--skip-download", "--no-warnings", "--quiet", "--impersonate", "chrome"}
	args = append(args, d.extraArgs()...)
	args = append(args, d.cookieArgs()...)
	args = append(args, d.refererArgs()...)
	args = append(args, resolved)

	out, err := d.run(ctx, "yt-dlp", args...)
	if err != nil {
		return nil, resolved, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	var info map[string]any
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, resolved, err
	}
	return info, resolved, nil
}

func (d *Downloader) runYTDLP(ctx context.Context, url, outputDir, format string) error {
	args := []string{
		"--no-warnings", "--quiet",
		"--impersonate", "chrome",
		"-o", filepath.Join(outputDir, "%(title).100s_%(id)s.%(ext)s"),
		"-f", format,
	}
	args = append(args, d.extraArgs()...)
	args = append(args, d.cookieArgs()...)
	args = append(args, d.refererArgs()...)
	args = append(args, url)
	out, err := d.run(ctx, "yt-dlp", args...)
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ensureAudio detects TikTok renditions that yt-dlp's format metadata
// falsely advertises as having audio but which actually contain a video
// track only (a known TikTok extractor quirk). When that happens it falls
// back to the watermarked "download" rendition, which reliably carries
// real audio, and mixes that audio into the original (unwatermarked)
// video rather than settling for the lower-quality watermarked copy.
func (d *Downloader) ensureAudio(ctx context.Context, videoPath, url, outputDir string) (string, error) {
	hasAudio, err := video.HasAudioStream(ctx, videoPath)
	if err != nil || hasAudio {
		return videoPath, nil
	}

	wmDir := filepath.Join(outputDir, "wm_audio_src")
	if err := os.MkdirAll(wmDir, 0o755); err != nil {
		return videoPath, err
	}
	defer os.RemoveAll(wmDir)

	if err := d.runYTDLP(ctx, url, wmDir, "download"); err != nil {
		return videoPath, err
	}
	_, wmVideo, _ := findMediaFiles(wmDir)
	if wmVideo == "" {
		return videoPath, fmt.Errorf("no audio source found")
	}
	if wmHasAudio, err := video.HasAudioStream(ctx, wmVideo); err != nil || !wmHasAudio {
		return videoPath, fmt.Errorf("fallback source has no audio")
	}

	return video.MuxAudioFrom(ctx, videoPath, wmVideo)
}

func (d *Downloader) downloadAudio(ctx context.Context, url, outputDir string) (string, error) {
	args := []string{
		"--no-warnings", "--quiet",
		"--impersonate", "chrome",
		"-f", "bestaudio/best",
		"-o", filepath.Join(outputDir, "audio.%(ext)s"),
		"--extract-audio", "--audio-format", "mp3",
	}
	args = append(args, d.extraArgs()...)
	args = append(args, d.cookieArgs()...)
	args = append(args, d.refererArgs()...)
	args = append(args, url)
	out, err := d.run(ctx, "yt-dlp", args...)
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	_ = out
	for _, ext := range []string{".mp3", ".m4a", ".aac", ".opus"} {
		matches, _ := filepath.Glob(filepath.Join(outputDir, "*"+ext))
		if len(matches) > 0 {
			return matches[0], nil
		}
	}
	return "", fmt.Errorf("no audio file")
}

func (d *Downloader) downloadImages(ctx context.Context, urls []string, outputDir string) []string {
	if d.maxImages > 0 && len(urls) > d.maxImages {
		urls = urls[:d.maxImages]
	}
	var paths []string
	client := &http.Client{Timeout: 15 * time.Second}
	for i, imgURL := range urls {
		ext := guessImageExt(imgURL)
		path := filepath.Join(outputDir, fmt.Sprintf("image_%04d%s", i, ext))
		if err := downloadHTTP(ctx, client, imgURL, path); err == nil {
			paths = append(paths, path)
		}
	}
	return paths
}

func (d *Downloader) cookieArgs() []string {
	if d.cookiesPath != "" {
		return []string{"--cookies", d.cookiesPath}
	}
	if d.cookiesFromBrowser != "" {
		return []string{"--cookies-from-browser", d.cookiesFromBrowser}
	}
	return nil
}

func (d *Downloader) refererArgs() []string {
	referer := strings.TrimSpace(d.referer)
	if referer == "" {
		referer = TikTokRefererDefault
	}
	return []string{"--referer", referer}
}

func (d *Downloader) extraArgs() []string {
	var args []string
	args = append(args, "--extractor-args", TikTokPlayerClient)
	if d.maxDuration > 0 {
		args = append(args, "--max-duration", fmt.Sprintf("%d", d.maxDuration))
	}
	return args
}

func (d *Downloader) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

func resolvePageURL(url string) string {
	// For full www.tiktok.com/@user/video/<id> URLs normalize to a canonical
	// form; for short links (vt.tiktok.com, vm.tiktok.com) keep the original
	// URL and let yt-dlp resolve the redirect with --impersonate + --referer.
	// The previous implementation tried to follow vt redirects with a plain
	// Go http.Client (no impersonation, no Referer) which TikTok's CDN now
	// blocks on datacenter IPs with a bot-challenge page ("Unexpected response
	// from webpage request"). Letting yt-dlp handle it is both correct and
	// more resilient — verified locally that yt-dlp 2026.07.04 already resolves
	// vt.tiktok.com/ZSVwhXM2d via its own extractor when given --referer.
	if pageItemRE.MatchString(url) {
		return canonicalVideoURL(url)
	}
	if m := itemIDRE.FindStringSubmatch(url); len(m) == 2 && strings.Contains(url, "tiktok.com/") {
		return canonicalVideoURL(url)
	}
	// Short links or other forms: strip query/fragment but don't try to fetch.
	return strings.SplitN(url, "?", 2)[0]
}

func canonicalVideoURL(url string) string {
	if m := pageItemRE.FindStringSubmatch(url); len(m) == 4 {
		return fmt.Sprintf("https://www.tiktok.com/@%s/video/%s", m[1], m[3])
	}
	if m := itemIDRE.FindStringSubmatch(url); len(m) == 2 {
		return fmt.Sprintf("https://www.tiktok.com/@/video/%s", m[1])
	}
	return strings.SplitN(url, "?", 2)[0]
}

func extractCarouselURLs(info map[string]any) []string {
	var urls []string
	entries := infoEntries(info)

	// For photomode/slideshow posts, top-level thumbnails may contain images
	// when entries are not present.
	if len(entries) == 0 {
		for _, t := range infoThumbnails(info) {
			if u := stringField(t, "url"); u != "" && !looksLikeVideoURL(u) {
				urls = appendUnique(urls, u)
			}
		}
		return urls
	}

	for _, entry := range entries {
		if thumb := stringField(entry, "thumbnail"); thumb != "" && !looksLikeVideoURL(thumb) {
			urls = appendUnique(urls, thumb)
			continue
		}
		if thumbs, ok := entry["thumbnails"].([]any); ok && len(thumbs) > 0 {
			if m, ok := thumbs[0].(map[string]any); ok {
				if u := stringField(m, "url"); u != "" && !looksLikeVideoURL(u) {
					urls = appendUnique(urls, u)
				}
			}
		}
		if u := stringField(entry, "url"); u != "" && !looksLikeVideoURL(u) {
			urls = appendUnique(urls, u)
		}
	}
	return urls
}

func preferVideoDelivery(info map[string]any) bool {
	vcodec := stringField(info, "vcodec")
	if stringField(info, "url") != "" && vcodec != "" && vcodec != "none" {
		return true
	}
	for _, entry := range infoEntries(info) {
		if stringField(entry, "vcodec") != "" && stringField(entry, "vcodec") != "none" {
			return true
		}
	}
	return false
}

func metadataFromInfo(info map[string]any) (title, author string) {
	desc := stringField(info, "description")
	rawTitle := stringField(info, "title")
	author = firstNonEmpty(stringField(info, "uploader"), stringField(info, "creator"))
	if strings.TrimSpace(desc) != "" {
		return strings.TrimSpace(desc), author
	}
	if strings.TrimSpace(rawTitle) != "" && !strings.HasPrefix(strings.ToLower(rawTitle), "tiktok video #") {
		return strings.TrimSpace(rawTitle), author
	}
	return "", author
}

func infoEntries(info map[string]any) []map[string]any {
	raw, ok := info["entries"].([]any)
	if !ok {
		return nil
	}
	var out []map[string]any
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func infoThumbnails(info map[string]any) []map[string]any {
	raw, ok := info["thumbnails"].([]any)
	if !ok {
		return nil
	}
	var out []map[string]any
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func isPhotoMode(info map[string]any) bool {
	for _, t := range infoThumbnails(info) {
		if strings.Contains(stringField(t, "url"), "photomode") {
			return true
		}
	}
	return false
}

func detectPostType(images []string, video, audio string) PostType {
	if video != "" {
		return PostTypeVideo
	}
	if len(images) > 0 {
		return PostTypeCarousel
	}
	if audio != "" {
		return PostTypeAudioOnly
	}
	return PostTypeUnknown
}

func findMediaFiles(dir string) (images []string, video, audio string) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		ext := strings.ToLower(filepath.Ext(e.Name()))
		switch ext {
		case ".jpg", ".jpeg", ".png", ".webp":
			images = append(images, path)
		case ".mp4", ".webm", ".mov", ".mkv", ".m4v":
			video = path
		case ".mp3", ".m4a", ".opus", ".aac", ".wav", ".flac", ".ogg":
			audio = path
		}
	}
	return images, video, audio
}

func writableCookies(path string) string {
	return cookies.SyncFromMount(path, cookies.TikTokWritablePath)
}

func downloadHTTP(ctx context.Context, client *http.Client, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func looksLikeVideoURL(url string) bool {
	lower := strings.ToLower(url)
	if strings.Contains(lower, "/video/") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(strings.SplitN(url, "?", 2)[0]))
	switch ext {
	case ".mp4", ".webm", ".mov", ".mkv", ".m4v":
		return true
	}
	return false
}

func guessImageExt(url string) string {
	ext := strings.ToLower(filepath.Ext(strings.SplitN(url, "?", 2)[0]))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		return ext
	default:
		return ".jpg"
	}
}

func appendUnique(list []string, value string) []string {
	for _, v := range list {
		if v == value {
			return list
		}
	}
	return append(list, value)
}

func stringField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func boolField(m map[string]any, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
