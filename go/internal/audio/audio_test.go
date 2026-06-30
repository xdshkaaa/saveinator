package audio

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsDRMProtectedError(t *testing.T) {
	t.Parallel()
	err := errors.New("exit status 1")
	if !IsDRMProtectedError(err, "ERROR: [soundcloud] DRM protected") {
		t.Fatal("expected drm error")
	}
	if IsDRMProtectedError(errors.New("not found"), "HTTP 404") {
		t.Fatal("expected non-drm")
	}
}

func TestFindAudioFilePicksNewest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.mp3")
	newPath := filepath.Join(dir, "new.mp3")
	oldTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	newTime := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := os.WriteFile(oldPath, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newPath, newTime, newTime); err != nil {
		t.Fatal(err)
	}

	got, err := findAudioFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != newPath {
		t.Fatalf("findAudioFile = %q, want %q", got, newPath)
	}
}

func TestFindAudioFileIgnoresNonAudio(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := findAudioFile(dir); err == nil {
		t.Fatal("expected error when no audio files")
	}
}

func TestFindAudioFileEmptyDir(t *testing.T) {
	t.Parallel()
	if _, err := findAudioFile(t.TempDir()); err == nil {
		t.Fatal("expected error for empty dir")
	}
}
