package ytdlp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Options struct {
	FormatID                    string
	Platform                    string
	InstagramCookies            string
	InstagramCookiesFromBrowser string
	TikTokCookies               string
	TikTokCookiesFromBrowser    string
	Timeout                     time.Duration
}

func Download(ctx context.Context, url string, outputDir string, opts Options) error {
	return run(ctx, url, outputDir, opts, false)
}

func Probe(ctx context.Context, url string, outputDir string, opts Options) error {
	return run(ctx, url, outputDir, opts, true)
}

func run(ctx context.Context, url string, outputDir string, opts Options, skipDownload bool) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}

	args := []string{
		"--quiet",
		"--no-warnings",
		"--no-playlist",
		"-o", filepath.Join(outputDir, "%(title).100s_%(id)s.%(ext)s"),
	}

	if opts.FormatID != "" && !skipDownload {
		args = append(args, "-f", opts.FormatID, "--merge-output-format", "mp4")
	} else if !skipDownload {
		args = append(args, "-f", "best")
	}
	if skipDownload {
		args = append(args, "--skip-download")
	}

	args = appendPlatformCookies(args, prepareCookieOptions(outputDir, opts))

	args = append(args, url)

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	cmd.Env = append(os.Environ(),
		"PYTHONUNBUFFERED=1",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("yt-dlp failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}


func writableCookiesPath(src, workDir string) (string, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	dest := filepath.Join(workDir, "yt-dlp-cookies.txt")
	if err := os.WriteFile(dest, data, 0o600); err != nil {
		return "", err
	}
	return dest, nil
}

func prepareCookieOptions(outputDir string, opts Options) Options {
	prepared := opts
	if fileExists(prepared.InstagramCookies) {
		if dest, err := writableCookiesPath(prepared.InstagramCookies, outputDir); err == nil {
			prepared.InstagramCookies = dest
		}
	}
	if fileExists(prepared.TikTokCookies) {
		if dest, err := writableCookiesPath(prepared.TikTokCookies, outputDir); err == nil {
			prepared.TikTokCookies = dest
		}
	}
	return prepared
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Size() > 0
}

func appendPlatformCookies(args []string, opts Options) []string {
	switch opts.Platform {
	case "instagram":
		if fileExists(opts.InstagramCookies) {
			return append(args, "--cookies", opts.InstagramCookies)
		}
		if browser := strings.TrimSpace(opts.InstagramCookiesFromBrowser); browser != "" {
			return append(args, "--cookies-from-browser", browser)
		}
	case "tiktok":
		if fileExists(opts.TikTokCookies) {
			return append(args, "--cookies", opts.TikTokCookies)
		}
		if browser := strings.TrimSpace(opts.TikTokCookiesFromBrowser); browser != "" {
			return append(args, "--cookies-from-browser", browser)
		}
	}
	return args
}

var (
	videoExtensions = map[string]struct{}{
		".mp4": {}, ".webm": {}, ".mkv": {}, ".mov": {}, ".m4v": {},
	}
	imageExtensions = map[string]struct{}{
		".jpg": {}, ".jpeg": {}, ".png": {}, ".webp": {},
	}
)

func FindMediaFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if _, ok := videoExtensions[ext]; ok {
			files = append(files, filepath.Join(dir, e.Name()))
			continue
		}
		if _, ok := imageExtensions[ext]; ok {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files, nil
}

func LargestVideo(files []string) string {
	var best string
	var bestSize int64
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f))
		if _, ok := videoExtensions[ext]; !ok {
			continue
		}
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		if info.Size() > bestSize {
			best = f
			bestSize = info.Size()
		}
	}
	return best
}

func ImageFiles(files []string) []string {
	var images []string
	for _, f := range files {
		if _, ok := imageExtensions[strings.ToLower(filepath.Ext(f))]; ok {
			images = append(images, f)
		}
	}
	return images
}

func HasAudioStream(path string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "a",
		"-show_entries", "stream=index",
		"-of", "csv=p=0",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return true
	}
	return strings.TrimSpace(string(out)) != ""
}
