package video

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ProcessingError struct {
	Msg string
}

func (e ProcessingError) Error() string {
	return e.Msg
}

var ratioDimensions = map[string]map[int][2]int{
	"16_9": {1080: {1920, 1080}, 720: {1280, 720}, 480: {854, 480}, 360: {640, 360}, 240: {426, 240}, 144: {256, 144}},
	"21_9": {1080: {2560, 1080}, 720: {1680, 720}, 480: {1120, 480}, 360: {840, 360}, 240: {560, 240}, 144: {336, 144}},
	"9_16": {1080: {1080, 1920}, 720: {720, 1280}, 480: {480, 854}, 360: {360, 640}, 240: {240, 426}, 144: {144, 256}},
}

func ApplyAspectRatio(ctx context.Context, sourcePath, aspectRatio string, quality int) (string, error) {
	ratioMap, ok := ratioDimensions[aspectRatio]
	if !ok {
		return "", ProcessingError{Msg: "unsupported aspect ratio: " + aspectRatio}
	}
	dims, ok := ratioMap[quality]
	if !ok {
		return "", ProcessingError{Msg: fmt.Sprintf("unsupported quality for ratio: %d", quality)}
	}
	width, height := dims[0], dims[1]

	ext := filepath.Ext(sourcePath)
	outputPath := strings.TrimSuffix(sourcePath, ext) + "_" + aspectRatio + ".mp4"

	if probed, err := probeDimensions(ctx, sourcePath); err == nil && probed[0] == width && probed[1] == height {
		if sourcePath == outputPath {
			return sourcePath, nil
		}
		if err := runFFmpeg(ctx, []string{
			"ffmpeg", "-y", "-i", sourcePath,
			"-c", "copy", "-movflags", "+faststart", outputPath,
		}); err != nil {
			return "", err
		}
		return outputPath, nil
	}

	vf := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d", width, height, width, height)
	base := []string{
		"ffmpeg", "-y", "-i", sourcePath,
		"-vf", vf,
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "28",
		"-threads", "1", "-movflags", "+faststart",
	}
	if err := runFFmpeg(ctx, append(base, "-c:a", "copy", outputPath)); err != nil {
		if err := runFFmpeg(ctx, append(base, "-c:a", "aac", "-b:a", "128k", outputPath)); err != nil {
			return "", err
		}
	}
	return outputPath, nil
}

// bitrateLadderKbps gives a reasonable target video bitrate (kbps) per
// resolution height, above which a source is considered over-encoded for
// its resolution (common for long YouTube uploads with high source bitrate).
var bitrateLadderKbps = map[int]int{
	2160: 12000,
	1440: 8000,
	1080: 4500,
	720:  2500,
	480:  1200,
	360:  700,
}

func targetBitrateKbps(height int) int {
	best := 0
	bestDiff := -1
	for h, kbps := range bitrateLadderKbps {
		diff := h - height
		if diff < 0 {
			diff = -diff
		}
		if bestDiff == -1 || diff < bestDiff {
			bestDiff = diff
			best = kbps
		}
	}
	return best
}

// CompressIfOversized re-encodes long/high-bitrate videos down toward a
// resolution-appropriate target bitrate using a slower x264 preset, which
// yields meaningfully smaller files at visually equivalent quality compared
// to the source's original encode. Videos already within the target
// bitrate (e.g. already-efficient vp9/av01 downloads) are left untouched.
func CompressIfOversized(ctx context.Context, sourcePath string, minDurationSec int) (string, error) {
	duration, err := probeDurationSeconds(ctx, sourcePath)
	if err != nil || duration < float64(minDurationSec) {
		return sourcePath, nil
	}

	dims, err := probeDimensions(ctx, sourcePath)
	if err != nil {
		return sourcePath, nil
	}

	info, err := os.Stat(sourcePath)
	if err != nil {
		return sourcePath, nil
	}

	currentKbps := float64(info.Size()*8) / duration / 1000
	target := targetBitrateKbps(dims[1])

	// Only re-encode when the source is meaningfully above target; avoids
	// wasted CPU/time on files that are already efficiently encoded.
	if currentKbps <= float64(target)*1.2 {
		return sourcePath, nil
	}

	ext := filepath.Ext(sourcePath)
	base := strings.TrimSuffix(sourcePath, ext)
	tmpPath := base + "_compressed_tmp.mp4"
	finalPath := base + ".mp4"

	maxrate := target * 12 / 10
	bufsize := target * 2
	err = runFFmpegWithTimeout(ctx, []string{
		"ffmpeg", "-y", "-i", sourcePath,
		"-c:v", "libx264", "-preset", "slow", "-crf", "23",
		"-maxrate", fmt.Sprintf("%dk", maxrate), "-bufsize", fmt.Sprintf("%dk", bufsize),
		"-c:a", "copy", "-movflags", "+faststart", tmpPath,
	}, 20*time.Minute)
	if err != nil {
		_ = os.Remove(tmpPath)
		return sourcePath, nil
	}

	// Swap the compressed file into the source's filename (title/metadata
	// parsing elsewhere keys off the filename, so it must not gain a
	// distinguishing suffix like the temp name above).
	if finalPath != sourcePath {
		if err := os.Remove(sourcePath); err != nil {
			_ = os.Remove(tmpPath)
			return sourcePath, nil
		}
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return sourcePath, nil
	}
	return finalPath, nil
}

func probeDurationSeconds(ctx context.Context, path string) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-show_entries", "format=duration",
		"-of", "csv=p=0", path,
	).Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}

// Probe returns the video's pixel width, height, and duration in seconds.
func Probe(ctx context.Context, path string) (width, height int, durationSec float64, err error) {
	dims, err := probeDimensions(ctx, path)
	if err != nil {
		return 0, 0, 0, err
	}
	duration, err := probeDurationSeconds(ctx, path)
	if err != nil {
		duration = 0
	}
	return dims[0], dims[1], duration, nil
}

// GenerateThumbnail extracts a representative frame and scales it to fit
// Telegram's thumbnail constraints (JPEG, <=320px per side). Telegram will
// generate its own preview when none is supplied, but that server-side
// generation has been observed to produce a visibly stretched preview for
// some TikTok renditions even though the underlying video itself plays
// back with the correct aspect ratio — supplying a correct thumbnail
// ourselves avoids relying on it.
func GenerateThumbnail(ctx context.Context, videoPath string) (string, error) {
	ext := filepath.Ext(videoPath)
	thumbPath := strings.TrimSuffix(videoPath, ext) + "_thumb.jpg"

	err := runFFmpegWithTimeout(ctx, []string{
		"ffmpeg", "-y", "-ss", "00:00:00.1", "-i", videoPath,
		"-frames:v", "1",
		"-vf", "scale='min(320,iw)':'min(320,ih)':force_original_aspect_ratio=decrease",
		"-q:v", "4",
		thumbPath,
	}, 30*time.Second)
	if err != nil {
		return "", err
	}
	return thumbPath, nil
}

// HasAudioStream reports whether the file has at least one audio stream.
func HasAudioStream(ctx context.Context, path string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-select_streams", "a",
		"-show_entries", "stream=codec_type",
		"-of", "csv=p=0", path,
	).Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// MuxAudioFrom replaces videoPath's audio track (or adds one if missing)
// with the audio from audioSourcePath, keeping videoPath's video stream
// untouched. Used when a preferred (e.g. unwatermarked) video rendition
// lacks audio but another rendition of the same source has it.
func MuxAudioFrom(ctx context.Context, videoPath, audioSourcePath string) (string, error) {
	ext := filepath.Ext(videoPath)
	outputPath := strings.TrimSuffix(videoPath, ext) + "_muxed.mp4"

	err := runFFmpegWithTimeout(ctx, []string{
		"ffmpeg", "-y", "-i", videoPath, "-i", audioSourcePath,
		"-map", "0:v:0", "-map", "1:a:0",
		"-c:v", "copy", "-c:a", "copy", "-shortest",
		"-movflags", "+faststart", outputPath,
	}, 3*time.Minute)
	if err != nil {
		return videoPath, err
	}

	if rmErr := os.Remove(videoPath); rmErr != nil {
		_ = os.Remove(outputPath)
		return videoPath, rmErr
	}
	finalPath := strings.TrimSuffix(videoPath, ext) + ".mp4"
	if renErr := os.Rename(outputPath, finalPath); renErr != nil {
		return videoPath, renErr
	}
	return finalPath, nil
}

func probeDimensions(ctx context.Context, path string) ([2]int, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "csv=p=0:s=x", path,
	).Output()
	if err != nil {
		return [2]int{}, err
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "x")
	if len(parts) != 2 {
		return [2]int{}, fmt.Errorf("invalid ffprobe output")
	}
	w, err1 := strconv.Atoi(parts[0])
	h, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return [2]int{}, fmt.Errorf("invalid dimensions")
	}
	return [2]int{w, h}, nil
}

// NormalizeSAR fixes videos whose container declares a non-square sample
// aspect ratio (SAR != 1:1). Telegram's clients render raw pixel dimensions
// as square pixels and ignore the SAR tag, so a source that relies on SAR
// to reach its intended display ratio (common with TikTok's h264 renditions)
// plays back visibly stretched/squashed. Baking the SAR into actual pixel
// width via scale+setsar makes the file correct regardless of SAR support.
func NormalizeSAR(ctx context.Context, sourcePath string) (string, error) {
	sar, err := probeSAR(ctx, sourcePath)
	if err != nil || sar == "" || sar == "0:1" || sar == "1:1" || sar == "N/A" {
		return sourcePath, nil
	}

	ext := filepath.Ext(sourcePath)
	outputPath := strings.TrimSuffix(sourcePath, ext) + "_sar1.mp4"

	err = runFFmpegWithTimeout(ctx, []string{
		"ffmpeg", "-y", "-i", sourcePath,
		"-vf", "scale=trunc(iw*sar/2)*2:ih,setsar=1",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "20",
		"-c:a", "copy", "-movflags", "+faststart", outputPath,
	}, 3*time.Minute)
	if err != nil {
		return sourcePath, nil
	}

	if rmErr := os.Remove(sourcePath); rmErr != nil {
		_ = os.Remove(outputPath)
		return sourcePath, nil
	}
	finalPath := strings.TrimSuffix(sourcePath, ext) + ".mp4"
	if renErr := os.Rename(outputPath, finalPath); renErr != nil {
		return sourcePath, nil
	}
	return finalPath, nil
}

func probeSAR(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=sample_aspect_ratio",
		"-of", "csv=p=0", path,
	).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func runFFmpeg(ctx context.Context, args []string) error {
	return runFFmpegWithTimeout(ctx, args, 5*time.Minute)
}

func runFFmpegWithTimeout(ctx context.Context, args []string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			detail = err.Error()
		}
		if len(detail) > 500 {
			detail = detail[:500]
		}
		return ProcessingError{Msg: detail}
	}
	if _, err := os.Stat(args[len(args)-1]); err != nil {
		return ProcessingError{Msg: "ffmpeg produced no output file"}
	}
	return nil
}
