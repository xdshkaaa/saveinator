package audio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func IsDRMProtectedError(err error, outputs ...string) bool {
	if err == nil && len(outputs) == 0 {
		return false
	}
	msg := strings.ToLower(combineErrorText(err, outputs))
	return strings.Contains(msg, "drm protected")
}

func combineErrorText(err error, outputs []string) string {
	var b strings.Builder
	for _, o := range outputs {
		b.WriteString(o)
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			b.Write(exitErr.Stderr)
		}
		b.WriteString(err.Error())
	}
	return b.String()
}

const maxSearchAttempts = 3

// DownloadFromYouTubeSearch finds candidates via a flat ytsearch, ranks them by
// closeness to durationMS (when > 0), and tries them in order until one
// downloads. timeoutSeconds bounds search plus all download attempts combined.
func DownloadFromYouTubeSearch(ctx context.Context, query, outputDir, format string, durationMS, timeoutSeconds int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	candidates, err := resolveYouTubeSearch(ctx, query, durationMS)
	if err != nil {
		return "", err
	}

	var lastErr error
	for i, c := range candidates {
		if i >= maxSearchAttempts {
			break
		}
		path, dlErr := DownloadYouTubeAudio(ctx, "https://www.youtube.com/watch?v="+c.ID, outputDir, format, "")
		if dlErr == nil {
			return path, nil
		}
		lastErr = dlErr
		if ctx.Err() != nil {
			break
		}
		slog.Debug("youtube candidate failed, trying next", "query", query, "video_id", c.ID, "err", dlErr)
	}
	return "", fmt.Errorf("all youtube candidates failed for %q: %w", query, lastErr)
}

type searchCandidate struct {
	ID       string
	Duration float64
}

func resolveYouTubeSearch(ctx context.Context, query string, durationMS int) ([]searchCandidate, error) {
	out, err := exec.CommandContext(ctx, "yt-dlp",
		"--dump-single-json", "--flat-playlist", "--no-warnings", "--quiet",
		"ytsearch5:"+query,
	).Output()
	if err != nil {
		return nil, fmt.Errorf("youtube search failed: %s", combineErrorText(err, nil))
	}
	candidates := pickSearchCandidates(out, durationMS)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no youtube match for query")
	}
	return candidates, nil
}

func pickSearchCandidates(jsonBytes []byte, durationMS int) []searchCandidate {
	var info struct {
		Entries []*struct {
			ID       string  `json:"id"`
			Duration float64 `json:"duration"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(jsonBytes, &info); err != nil {
		return nil
	}
	var candidates []searchCandidate
	for _, e := range info.Entries {
		if e == nil || e.ID == "" {
			continue
		}
		candidates = append(candidates, searchCandidate{ID: e.ID, Duration: e.Duration})
	}
	if durationMS > 0 {
		target := float64(durationMS) / 1000
		sort.SliceStable(candidates, func(i, j int) bool {
			return math.Abs(candidates[i].Duration-target) < math.Abs(candidates[j].Duration-target)
		})
	}
	return candidates
}

// DownloadYouTubeAudio extracts the soundtrack of a YouTube video. section, when
// set, is a yt-dlp --download-sections range ("*12.00-45.00") limiting the fetch
// to a fragment.
func DownloadYouTubeAudio(ctx context.Context, url, outputDir, format, section string) (string, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}
	if format == "" {
		format = "mp3"
	}
	args := []string{
		"--no-warnings", "--quiet", "--no-playlist",
		"-f", "bestaudio/best",
		"-o", filepath.Join(outputDir, "%(title).100s.%(ext)s"),
		"--extract-audio", "--audio-format", format,
	}
	if section != "" {
		args = append(args, "--download-sections", section, "--force-keyframes-at-cuts")
	}
	out, err := exec.CommandContext(ctx, "yt-dlp", append(args, url)...).CombinedOutput()
	if err != nil {
		return "", ytdlpCombinedError(out, err)
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
	out, err := exec.CommandContext(ctx, "yt-dlp",
		"--no-warnings", "--quiet", "--no-playlist",
		"-f", "bestaudio/best",
		"-o", filepath.Join(outputDir, "%(title).100s.%(ext)s"),
		"--extract-audio", "--audio-format", format,
		url,
	).CombinedOutput()
	if err != nil {
		return "", ytdlpCombinedError(out, err)
	}
	return findAudioFile(outputDir)
}

func ytdlpCombinedError(out []byte, err error) error {
	text := strings.TrimSpace(string(out))
	if text != "" {
		return fmt.Errorf("%s: %w", text, err)
	}
	return err
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
