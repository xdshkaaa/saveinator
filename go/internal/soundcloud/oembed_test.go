package soundcloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestParseOEmbedTitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in         string
		wantTitle  string
		wantArtist string
	}{
		{"Забавные игры by Убийцы", "Забавные игры", "Убийцы"},
		{"Track by Artist", "Track", "Artist"},
		{"No Artist Here", "No Artist Here", ""},
		{"Song by A by B", "Song by A", "B"},
	}
	for _, tc := range tests {
		title, artist := parseOEmbedTitle(tc.in)
		if title != tc.wantTitle || artist != tc.wantArtist {
			t.Fatalf("parseOEmbedTitle(%q) = (%q, %q), want (%q, %q)", tc.in, title, artist, tc.wantTitle, tc.wantArtist)
		}
	}
}

func TestFetchReleaseFromOEmbed(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oembed" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(oembedResponse{
			Title:        "Забавные игры by Убийцы",
			ThumbnailURL: "https://example.com/art.jpg",
		})
	}))
	defer server.Close()

	origClient := oembedHTTPClient
	oembedHTTPClient = server.Client()
	t.Cleanup(func() { oembedHTTPClient = origClient })

	origAPI := oembedAPI
	t.Cleanup(func() { _ = origAPI })

	release, err := fetchReleaseFromOEmbedWithEndpoint(context.Background(), "https://soundcloud.com/ubiitsy/zabavnye-igry", server.URL+"/oembed")
	if err != nil {
		t.Fatal(err)
	}
	if release.Title != "Забавные игры" || release.Artist != "Убийцы" {
		t.Fatalf("release = %+v", release)
	}
	if len(release.Tracks) != 1 || !release.Tracks[0].YouTubeFallback {
		t.Fatalf("tracks = %+v", release.Tracks)
	}
	if release.ArtworkURL != "https://example.com/art.jpg" {
		t.Fatalf("artwork = %q", release.ArtworkURL)
	}
}

func TestReleaseUsesYouTubeFallback(t *testing.T) {
	t.Parallel()
	if !releaseUsesYouTubeFallback(&Release{Tracks: []Track{{YouTubeFallback: true}}}) {
		t.Fatal("expected true")
	}
	if releaseUsesYouTubeFallback(&Release{Tracks: []Track{{}}}) {
		t.Fatal("expected false")
	}
}

func fetchReleaseFromOEmbedWithEndpoint(ctx context.Context, pageURL, apiBase string) (*Release, error) {
	canonical := normalizeURL(pageURL)
	reqURL := apiBase + "?format=json&url=" + url.QueryEscape(canonical)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := oembedHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload oembedResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	title, artist := parseOEmbedTitle(payload.Title)
	track := Track{
		Title:           title,
		Artist:          artist,
		SoundCloudURL:   canonical,
		ArtworkURL:      payload.ThumbnailURL,
		TrackNumber:     1,
		YouTubeFallback: true,
	}
	return &Release{
		Title:         title,
		Artist:        artist,
		ReleaseType:   "track",
		ArtworkURL:    payload.ThumbnailURL,
		SoundCloudURL: canonical,
		Tracks:        []Track{track},
	}, nil
}
