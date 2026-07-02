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

type Downloader struct {
	cookiesPath          string
	cookiesFromBrowser   string
	timeout              time.Duration
	maxImages            int
	audioEnabled         bool
}

func NewDownloader(cookiesPath, cookiesFromBrowser string, timeoutSeconds, maxImages int, audioEnabled bool) *Downloader {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &Downloader{
		cookiesPath:        writableCookies(cookiesPath),
		cookiesFromBrowser: strings.TrimSpace(cookiesFromBrowser),
		timeout:            timeout,
		maxImages:          maxImages,
		audioEnabled:       audioEnabled,
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
	isSlideshow := boolField(info, "is_slideshow") || len(infoEntries(info)) >= 2

	if isSlideshow && len(imageURLs) >= 2 && !preferVideoDelivery(info) {
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

	if err := d.runYTDLP(ctx, resolved, outputDir, "best"); err != nil {
		return nil, err
	}

	images, video, audio := findMediaFiles(outputDir)
	result.Images = images
	result.VideoPath = video
	result.AudioPath = audio
	result.PostType = detectPostType(images, video, audio)

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
	args := []string{"--dump-single-json", "--skip-download", "--no-warnings", "--quiet"}
	args = append(args, d.cookieArgs()...)
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
		"-o", filepath.Join(outputDir, "%(title).100s_%(id)s.%(ext)s"),
		"-f", format,
	}
	args = append(args, d.cookieArgs()...)
	args = append(args, url)
	out, err := d.run(ctx, "yt-dlp", args...)
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (d *Downloader) downloadAudio(ctx context.Context, url, outputDir string) (string, error) {
	args := []string{
		"--no-warnings", "--quiet",
		"-f", "bestaudio/best",
		"-o", filepath.Join(outputDir, "audio.%(ext)s"),
		"--extract-audio", "--audio-format", "mp3",
	}
	args = append(args, d.cookieArgs()...)
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

func (d *Downloader) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

func resolvePageURL(url string) string {
	if pageItemRE.MatchString(url) {
		return canonicalVideoURL(url)
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return canonicalVideoURL(url)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	}}
	resp, err := client.Do(req)
	if err != nil {
		return canonicalVideoURL(url)
	}
	resp.Body.Close()
	return canonicalVideoURL(resp.Request.URL.String())
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
	for _, entry := range infoEntries(info) {
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
	if stringField(info, "url") != "" {
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
