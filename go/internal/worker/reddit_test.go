package worker

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewestFileIn_skipsCookieScratch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	video := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(video, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	cookie := filepath.Join(dir, "yt-dlp-cookies.txt")
	if err := os.WriteFile(cookie, []byte("cookies"), 0o600); err != nil {
		t.Fatal(err)
	}
	// yt-dlp rewrites the cookie copy after the download, so its mtime wins.
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(cookie, future, future); err != nil {
		t.Fatal(err)
	}

	path, size, err := newestFileIn(dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "video.mp4" {
		t.Fatalf("picked %q, want video.mp4", path)
	}
	if size != 5 {
		t.Fatalf("size = %d, want 5", size)
	}
}
