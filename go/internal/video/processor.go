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
	"16_9": {1080: {1920, 1080}, 720: {1280, 720}, 480: {854, 480}},
	"21_9": {1080: {2560, 1080}, 720: {1680, 720}, 480: {1120, 480}},
	"9_16": {1080: {1080, 1920}, 720: {720, 1280}, 480: {480, 854}},
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

func runFFmpeg(ctx context.Context, args []string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
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
