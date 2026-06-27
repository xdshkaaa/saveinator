package spotify

import "errors"

var (
	ErrNotFound = errors.New("spotify resource not found")
	ErrAuth     = errors.New("spotify auth failed")
)

type Track struct {
	SourceID    string `json:"source_id"`
	Title       string `json:"title"`
	Artists     string `json:"artists"`
	DurationMS  int    `json:"duration_ms"`
	SpotifyURL  string `json:"spotify_url"`
	DiscNumber  int    `json:"disc_number"`
	TrackNumber int    `json:"track_number"`
}

type Release struct {
	SourceID    string  `json:"source_id"`
	Title       string  `json:"title"`
	Artists     string  `json:"artists"`
	AlbumType   string  `json:"album_type"`
	ReleaseDate string  `json:"release_date"`
	CoverURL    string  `json:"cover_url,omitempty"`
	SpotifyURL  string  `json:"spotify_url"`
	Tracks      []Track `json:"tracks"`
}

func normalizeAlbum(album map[string]any, items []map[string]any) *Release {
	albumID := stringField(album, "id")
	tracks := make([]Track, 0, len(items))
	for _, item := range items {
		tracks = append(tracks, normalizeTrack(item))
	}
	return &Release{
		SourceID:    albumID,
		Title:       stringField(album, "name"),
		Artists:     formatArtists(album["artists"]),
		AlbumType:   stringField(album, "album_type"),
		ReleaseDate: stringField(album, "release_date"),
		CoverURL:    firstImageURL(album["images"]),
		SpotifyURL:  externalURL(album, "album", albumID),
		Tracks:      tracks,
	}
}

func releaseFromTrack(apiTrack map[string]any) *Release {
	track := normalizeTrack(apiTrack)
	album, _ := apiTrack["album"].(map[string]any)
	cover := ""
	date := ""
	albumID := track.SourceID
	if album != nil {
		cover = firstImageURL(album["images"])
		date = stringField(album, "release_date")
		if id := stringField(album, "id"); id != "" {
			albumID = id
		}
	}
	return &Release{
		SourceID:    albumID,
		Title:       track.Title,
		Artists:     track.Artists,
		AlbumType:   "track",
		ReleaseDate: date,
		CoverURL:    cover,
		SpotifyURL:  track.SpotifyURL,
		Tracks:      []Track{track},
	}
}

func normalizeTrack(apiTrack map[string]any) Track {
	id := stringField(apiTrack, "id")
	return Track{
		SourceID:    id,
		Title:       stringField(apiTrack, "name"),
		Artists:     formatArtists(apiTrack["artists"]),
		DurationMS:  intField(apiTrack, "duration_ms"),
		SpotifyURL:  externalURL(apiTrack, "track", id),
		DiscNumber:  intField(apiTrack, "disc_number"),
		TrackNumber: intField(apiTrack, "track_number"),
	}
}

func formatArtists(raw any) string {
	items, ok := raw.([]any)
	if !ok {
		return ""
	}
	var names []string
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if name := stringField(m, "name"); name != "" {
			names = append(names, name)
		}
	}
	return stringsJoin(names, ", ")
}

func firstImageURL(raw any) string {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return ""
	}
	if m, ok := items[0].(map[string]any); ok {
		return stringField(m, "url")
	}
	return ""
}

func externalURL(data map[string]any, typ, id string) string {
	if urls, ok := data["external_urls"].(map[string]any); ok {
		if u := stringField(urls, "spotify"); u != "" {
			return u
		}
	}
	return "https://open.spotify.com/" + typ + "/" + id
}

func stringField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func intField(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func stringsJoin(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += sep + p
	}
	return out
}
