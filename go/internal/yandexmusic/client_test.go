package yandexmusic

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testServer(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return newClient("test-token", 5, srv.URL), srv
}

func TestFetchTrack(t *testing.T) {
	c, _ := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/tracks/154402671" {
			t.Errorf("path = %q", got)
		}
		if got := r.URL.RawQuery; got != "with-children=true" {
			t.Errorf("query = %q", got)
		}
		if auth := r.Header.Get("Authorization"); auth != "OAuth test-token" {
			t.Errorf("auth = %q", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": []map[string]any{{
				"id":         154402671,
				"title":      "Test Song",
				"durationMs": 213000,
				"coverUri":   "avatars.yandex.net/get-music-content/abc/def/%%",
				"artists":    []map[string]any{{"name": "Artist One"}, {"name": "Artist Two"}},
				"albums": []map[string]any{{
					"id":         43378588,
					"title":      "Great Album",
					"year":       2024,
					"trackCount": 12,
					"coverUri":   "",
				}},
			}},
		})
	})

	release, err := c.FetchRelease("", "154402671")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(release.Tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(release.Tracks))
	}
	track := release.Tracks[0]
	if track.SourceID != "154402671" || track.Title != "Test Song" || track.DurationMS != 213000 {
		t.Fatalf("track = %+v", track)
	}
	if track.Artists != "Artist One, Artist Two" {
		t.Fatalf("artists = %q", track.Artists)
	}
	if release.AlbumID != "43378588" || release.AlbumTitle != "Great Album" || release.AlbumTrackCount != 12 {
		t.Fatalf("album ref = %+v", release)
	}
	if release.ReleaseDate != "2024" {
		t.Fatalf("year = %q", release.ReleaseDate)
	}
	if release.CoverURL != "https://avatars.yandex.net/get-music-content/abc/def/400x400" {
		t.Fatalf("cover = %q", release.CoverURL)
	}
}

func TestFetchAlbumFlattensVolumes(t *testing.T) {
	c, _ := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/albums/43378588/with-tracks" {
			t.Errorf("path = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{
				"id":       43378588,
				"title":    "Great Album",
				"year":     2024,
				"coverUri": "//avatars.yandex.net/get-music-content/x/y/%%",
				"artists":  []map[string]any{{"name": "Artist One"}},
				"volumes": [][]map[string]any{
					{{"id": 1, "title": "T1", "durationMs": 100000}, {"id": 2, "title": "T2", "durationMs": 200000}},
					{{"id": 3, "title": "T3", "durationMs": 300000}},
				},
			},
		})
	})

	release, err := c.FetchRelease("43378588", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(release.Tracks) != 3 {
		t.Fatalf("expected 3 tracks, got %d", len(release.Tracks))
	}
	if release.Title != "Great Album" || release.AlbumID != "43378588" {
		t.Fatalf("release = %+v", release)
	}
	if release.CoverURL != "https://avatars.yandex.net/get-music-content/x/y/400x400" {
		t.Fatalf("cover = %q", release.CoverURL)
	}
}

func TestFetchTrackLinkAlbumFallback(t *testing.T) {
	c, _ := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": []map[string]any{{
				"id": 42, "title": "X", "durationMs": 1000,
				"albums": []map[string]any{},
			}},
		})
	})
	release, err := c.FetchRelease("777", "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if release.AlbumID != "777" {
		t.Fatalf("link album fallback failed: %+v", release)
	}
}

func TestAPIErrors(t *testing.T) {
	cases := []struct {
		status int
		want   error
	}{
		{http.StatusNotFound, ErrNotFound},
		{http.StatusUnauthorized, ErrAuth},
		{http.StatusForbidden, ErrAuth},
		{451, ErrGeo},
	}
	for _, tc := range cases {
		c, _ := testServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(`{"error":{"name":"x"}}`))
		})
		_, err := c.FetchRelease("1", "")
		if !errors.Is(err, tc.want) {
			t.Fatalf("status %d: got %v, want %v", tc.status, err, tc.want)
		}
	}
}

func TestEnabled(t *testing.T) {
	if NewClient("", 5).Enabled() {
		t.Fatal("empty token must be disabled")
	}
	if !NewClient("tok", 5).Enabled() {
		t.Fatal("token must be enabled")
	}
}

func TestCoverURLVariants(t *testing.T) {
	cases := map[string]string{
		"":                                                    "",
		"avatars.yandex.net/get-x/a/b/%%":                     "https://avatars.yandex.net/get-x/a/b/400x400",
		"https://avatars.yandex.net/get-x/a/b/%%":             "https://avatars.yandex.net/get-x/a/b/400x400",
		"//avatars.yandex.net/get-x/a/b/%%":                   "https://avatars.yandex.net/get-x/a/b/400x400",
	}
	for in, want := range cases {
		if got := coverURL(in); got != want {
			t.Errorf("coverURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCardTextSingleTrack(t *testing.T) {
	release := &Release{
		Title: "Song", Artists: "Artist", AlbumTitle: "Album",
		Tracks: []Track{{Title: "Song", Artists: "Artist", DurationMS: 61000}},
		YandexURL: "https://music.yandex.ru/track/1",
	}
	text := CardText(release, "en", true)
	for _, want := range []string{"Artist", "Song", "1:01"} {
		if !strings.Contains(text, want) {
			t.Errorf("card text missing %q:\n%s", want, text)
		}
	}
}

func TestOpenKeyboardAlbumButton(t *testing.T) {
	single := &Release{Tracks: []Track{{Title: "S"}}, AlbumID: "555", AlbumTrackCount: 12,
		YandexURL: "https://music.yandex.ru/track/9"}
	kb := OpenKeyboard(single, "en")
	if kb == nil || len(kb.InlineKeyboard) != 1 || len(kb.InlineKeyboard[0]) != 2 {
		t.Fatalf("expected 1 row with 2 buttons, got %+v", kb)
	}
	if got := kb.InlineKeyboard[0][1].CallbackData; got != AlbumCallbackPrefix+"555" {
		t.Fatalf("callback data = %q", got)
	}

	album := &Release{Tracks: make([]Track, 5), YandexURL: "https://music.yandex.ru/album/5"}
	kb = OpenKeyboard(album, "en")
	if kb == nil || len(kb.InlineKeyboard[0]) != 1 {
		t.Fatalf("album card must have only the open button, got %+v", kb)
	}

	noURL := &Release{Tracks: []Track{{}}}
	if kb := OpenKeyboard(noURL, "en"); kb != nil {
		t.Fatal("no url and no album → nil keyboard expected")
	}
}
