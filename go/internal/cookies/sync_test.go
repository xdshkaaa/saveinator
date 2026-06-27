package cookies

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSyncFromMount_copiesWhenMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	if err := os.WriteFile(src, []byte("cookies"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := SyncFromMount(src, dst)
	if got != dst {
		t.Fatalf("expected %q, got %q", dst, got)
	}
	data, err := os.ReadFile(dst)
	if err != nil || string(data) != "cookies" {
		t.Fatalf("dst not copied: %v %q", err, data)
	}
}

func TestSyncFromMount_keepsNewerWritableCopy(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	if err := os.WriteFile(src, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(src, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("warm"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := SyncFromMount(src, dst)
	if got != dst {
		t.Fatalf("expected writable dst, got %q", got)
	}
	data, err := os.ReadFile(dst)
	if err != nil || string(data) != "warm" {
		t.Fatalf("writable copy overwritten: %q", data)
	}
}

func TestSyncFromMount_refreshesWhenSourceNewer(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	if err := os.WriteFile(dst, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(dst, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("deployed"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := SyncFromMount(src, dst)
	if got != dst {
		t.Fatalf("expected %q, got %q", dst, got)
	}
	data, err := os.ReadFile(dst)
	if err != nil || string(data) != "deployed" {
		t.Fatalf("expected deployed cookies, got %q", data)
	}
}

func TestSyncFromMount_emptySource(t *testing.T) {
	t.Parallel()
	if got := SyncFromMount("", "/tmp/x"); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}
