package soundcloud

import "testing"

func TestParseLinkShortURL(t *testing.T) {
	t.Parallel()
	link, err := ParseLink("https://on.soundcloud.com/pEc6MFIKxzILKNzAXd")
	if err != nil {
		t.Fatal(err)
	}
	if link.Type != LinkTypeShort {
		t.Fatalf("type = %q, want short", link.Type)
	}
	if link.URL != "https://on.soundcloud.com/pEc6MFIKxzILKNzAXd" {
		t.Fatalf("url = %q", link.URL)
	}
}

func TestIsRelatedTracksPlaylist(t *testing.T) {
	t.Parallel()
	info := map[string]any{
		"id":    "soundcloud:system-playlists:personalized-tracks:1169193676:2325190835",
		"title": "Related tracks: Мюли",
	}
	if !isRelatedTracksPlaylist(info) {
		t.Fatal("expected related tracks playlist")
	}
}

func TestCollapseRelatedTracksToSeedTrack(t *testing.T) {
	t.Parallel()
	release := &Release{
		SourceID:      "soundcloud:system-playlists:personalized-tracks:1169193676:2325190835",
		Title:         "Related tracks: Мюли",
		Artist:        "SoundCloud",
		ReleaseType:   "playlist",
		SoundCloudURL: "https://on.soundcloud.com/pEc6MFIKxzILKNzAXd",
		Tracks: []Track{
			{
				SourceID:      "2325190835",
				Title:         "Мюли",
				Artist:        "lourenz",
				SoundCloudURL: "https://soundcloud.com/lourenz-320110234/myuli",
				DurationMS:    104046,
			},
			{
				SourceID:      "999",
				Title:         "Other track",
				SoundCloudURL: "https://soundcloud.com/other/track",
			},
		},
	}

	collapsed := collapseRelatedTracksToSeedTrack(release)
	if collapsed.ReleaseType != "track" {
		t.Fatalf("release type = %q, want track", collapsed.ReleaseType)
	}
	if len(collapsed.Tracks) != 1 {
		t.Fatalf("tracks = %d, want 1", len(collapsed.Tracks))
	}
	if collapsed.Title != "Мюли" {
		t.Fatalf("title = %q", collapsed.Title)
	}
	if collapsed.SoundCloudURL != "https://soundcloud.com/lourenz-320110234/myuli" {
		t.Fatalf("url = %q", collapsed.SoundCloudURL)
	}
}

func TestNormalizeReleaseRelatedTracksPlaylist(t *testing.T) {
	t.Parallel()
	info := map[string]any{
		"id":    "soundcloud:system-playlists:personalized-tracks:1169193676:2325190835",
		"title": "Related tracks: Мюли",
		"_type": "playlist",
		"entries": []any{
			map[string]any{
				"id":          "2325190835",
				"title":       "Мюли",
				"uploader":    "lourenz",
				"duration":    104.046,
				"webpage_url": "https://soundcloud.com/lourenz-320110234/myuli",
			},
			map[string]any{
				"id":          "999",
				"title":       "Other",
				"webpage_url": "https://soundcloud.com/other/track",
			},
		},
	}

	release := normalizeRelease(info, "https://on.soundcloud.com/pEc6MFIKxzILKNzAXd")
	if release.ReleaseType != "playlist" || len(release.Tracks) != 2 {
		t.Fatalf("unexpected normalized release: %+v", release)
	}

	if !isRelatedTracksPlaylist(info) {
		t.Fatal("expected related tracks detection")
	}
	collapsed := collapseRelatedTracksToSeedTrack(release)
	if collapsed.ReleaseType != "track" || len(collapsed.Tracks) != 1 {
		t.Fatalf("unexpected collapsed release: %+v", collapsed)
	}
}

func TestNormalizeTrackURLFallback(t *testing.T) {
	t.Parallel()
	track := normalizeTrack(map[string]any{
		"id":    "123",
		"title": "Test",
		"url":   "https://soundcloud.com/user/test-track",
	}, 1)
	if track.SoundCloudURL != "https://soundcloud.com/user/test-track" {
		t.Fatalf("url = %q", track.SoundCloudURL)
	}
}
