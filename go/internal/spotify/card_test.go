package spotify

import "testing"

func TestFormatDuration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		ms   int
		want string
	}{
		{ms: 0, want: "0:00"},
		{ms: 65000, want: "1:05"},
		{ms: 180000, want: "3:00"},
		{ms: -1000, want: "0:00"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			if got := formatDuration(tt.ms); got != tt.want {
				t.Fatalf("formatDuration(%d) = %q, want %q", tt.ms, got, tt.want)
			}
		})
	}
}

func TestCardTextSingleTrack(t *testing.T) {
	t.Parallel()
	release := &Release{
		AlbumType: "track",
		Tracks: []Track{
			{Title: "Song", Artists: "Band", DurationMS: 120000},
		},
	}
	text := CardText(release, "en", true)
	if text == "" {
		t.Fatal("expected card text")
	}
}

func TestCardTextAlbum(t *testing.T) {
	t.Parallel()
	release := &Release{
		Title:       "Album",
		Artists:     "Band",
		AlbumType:   "album",
		ReleaseDate: "2024-01-01",
		Tracks:      []Track{{Title: "A"}, {Title: "B"}},
	}
	text := CardText(release, "en", false)
	if text == "" {
		t.Fatal("expected card text")
	}
}

func TestAlbumTypeLabelFallback(t *testing.T) {
	t.Parallel()
	if got := albumTypeLabel("unknown_type", "en"); got != "unknown_type" {
		t.Fatalf("albumTypeLabel = %q", got)
	}
}
