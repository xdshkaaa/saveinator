package audio

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func DownloadFromYouTubeSearch(ctx context.Context, query, outputDir, format string, timeoutSeconds int) (string, error) {
	videoID, err := resolveYouTubeSearch(ctx, query, timeoutSeconds)
	if err != nil {
		return "", err
	}
	return downloadYouTubeAudio(ctx, "https://www.youtube.com/watch?v="+videoID, outputDir, format, timeoutSeconds)
}

func resolveYouTubeSearch(ctx context.Context, query string, timeoutSeconds int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "yt-dlp",
		"--dump-single-json", "--skip-download", "--no-warnings", "--quiet",
		"ytsearch1:"+query,
	).Output()
	if err != nil {
		return "", fmt.Errorf("youtube search failed: %w", err)
	}
	var info map[string]any
	if err := json.Unmarshal(out, &info); err != nil {
		return "", err
	}
	if entries, ok := info["entries"].([]any); ok && len(entries) > 0 {
		if m, ok := entries[0].(map[string]any); ok {
			if id, _ := m["id"].(string); id != "" {
				return id, nil
			}
		}
	}
	if id, _ := info["id"].(string); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("no youtube match for query")
}

func downloadYouTubeAudio(ctx context.Context, url, outputDir, format string, timeoutSeconds int) (string, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}
	if format == "" {
		format = "mp3"
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	_, err := exec.CommandContext(ctx, "yt-dlp",
		"--no-warnings", "--quiet", "--no-playlist",
		"-f", "bestaudio/best",
		"-o", filepath.Join(outputDir, "%(title).100s.%(ext)s"),
		"--extract-audio", "--audio-format", format,
		url,
	).CombinedOutput()
	if err != nil {
		return "", err
	}
	return findAudioFile(outputDir)
}

func DownloadSoundCloudTrack(ctx context.Context, url, outputDir, format string, timeoutSeconds int) (string, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}
	if format == "" {
		format = "mp3"
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	_, err := exec.CommandContext(ctx, "yt-dlp",
		"--no-warnings", "--quiet", "--no-playlist",
		"-f", "bestaudio/best",
		"-o", filepath.Join(outputDir, "%(title).100s.%(ext)s"),
		"--extract-audio", "--audio-format", format,
		url,
	).CombinedOutput()
	if err != nil {
		return "", err
	}
	return findAudioFile(outputDir)
}

func findAudioFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	exts := map[string]struct{}{
		".mp3": {}, ".m4a": {}, ".flac": {}, ".wav": {}, ".aac": {}, ".opus": {}, ".ogg": {},
	}
	var best string
	var bestTime int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if _, ok := exts[ext]; !ok {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Unix() >= bestTime {
			bestTime = info.ModTime().Unix()
			best = filepath.Join(dir, e.Name())
		}
	}
	if best == "" {
		return "", fmt.Errorf("no audio file found")
	}
	return best, nil
}
