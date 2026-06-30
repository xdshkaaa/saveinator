package worker

import (
	"testing"

	"saveinator/internal/soundcloud"
)

func TestSoundCloudYouTubeQuery(t *testing.T) {
	t.Parallel()
	release := soundcloud.Release{Artist: "Band", Title: "Album"}
	track := soundcloud.Track{Title: "Song", Artist: "Artist"}
	got := soundCloudYouTubeQuery(track, release)
	if got != "Artist - Song" {
		t.Fatalf("query = %q", got)
	}
	trackOnly := soundCloudYouTubeQuery(soundcloud.Track{Title: "Song"}, soundcloud.Release{})
	if trackOnly != "Song" {
		t.Fatalf("trackOnly = %q", trackOnly)
	}
}
