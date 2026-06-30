package spotify

import "testing"

func TestNormalizeAlbum(t *testing.T) {
	t.Parallel()
	album := map[string]any{
		"id":            "album1",
		"name":          "Test Album",
		"album_type":    "album",
		"release_date":  "2024-01-01",
		"artists":       []any{map[string]any{"name": "Artist A"}},
		"images":        []any{map[string]any{"url": "https://cover.example/a.jpg"}},
		"external_urls": map[string]any{"spotify": "https://open.spotify.com/album/album1"},
	}
	tracks := []map[string]any{
		{
			"id":           "track1",
			"name":         "Track One",
			"duration_ms":  180000,
			"disc_number":  float64(1),
			"track_number": float64(1),
			"artists":      []any{map[string]any{"name": "Artist A"}},
		},
	}

	release := normalizeAlbum(album, tracks)
	if release.SourceID != "album1" || release.Title != "Test Album" {
		t.Fatalf("release = %+v", release)
	}
	if release.Artists != "Artist A" || release.CoverURL != "https://cover.example/a.jpg" {
		t.Fatalf("artists/cover = %q / %q", release.Artists, release.CoverURL)
	}
	if len(release.Tracks) != 1 || release.Tracks[0].Title != "Track One" {
		t.Fatalf("tracks = %+v", release.Tracks)
	}
}

func TestReleaseFromTrack(t *testing.T) {
	t.Parallel()
	apiTrack := map[string]any{
		"id":           "track9",
		"name":         "Single",
		"duration_ms":  float64(95000),
		"artists":      []any{map[string]any{"name": "Solo"}},
		"external_urls": map[string]any{
			"spotify": "https://open.spotify.com/track/track9",
		},
		"album": map[string]any{
			"id":           "alb9",
			"release_date": "2023-06-15",
			"images":       []any{map[string]any{"url": "https://cover.example/t.jpg"}},
		},
	}

	release := releaseFromTrack(apiTrack)
	if release.AlbumType != "track" || release.SourceID != "alb9" {
		t.Fatalf("release = %+v", release)
	}
	if len(release.Tracks) != 1 || release.Tracks[0].DurationMS != 95000 {
		t.Fatalf("tracks = %+v", release.Tracks)
	}
}

func TestFormatArtists(t *testing.T) {
	t.Parallel()
	raw := []any{
		map[string]any{"name": "A"},
		map[string]any{"name": "B"},
	}
	if got := formatArtists(raw); got != "A, B" {
		t.Fatalf("formatArtists = %q", got)
	}
	if formatArtists(nil) != "" {
		t.Fatal("expected empty for nil")
	}
}

func TestExternalURLFallback(t *testing.T) {
	t.Parallel()
	got := externalURL(map[string]any{}, "track", "xyz")
	if got != "https://open.spotify.com/track/xyz" {
		t.Fatalf("externalURL = %q", got)
	}
}
