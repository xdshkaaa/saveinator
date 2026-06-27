package x

import (
	"path/filepath"
	"testing"
)

func TestMediaBasename_embeddedURL(t *testing.T) {
	path := "/tmp/saveinator-x/McGriller 🦆 - https://t.co/VELyrgEbTN_2070912405048332288.mp4"
	got := mediaBasename(path)
	want := "McGriller 🦆 - https://t.co/VELyrgEbTN_2070912405048332288.mp4"
	if got != want {
		t.Fatalf("mediaBasename() = %q, filepath.Base=%q, want %q", got, filepath.Base(path), want)
	}
}
