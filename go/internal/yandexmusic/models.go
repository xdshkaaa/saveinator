package yandexmusic

import (
	"fmt"
	"strconv"
	"strings"
)

// Track is a normalized track ready for the worker pipeline.
type Track struct {
	SourceID   string `json:"source_id"`
	Title      string `json:"title"`
	Artists    string `json:"artists"`
	DurationMS int    `json:"duration_ms"`
	YandexURL  string `json:"yandex_url"`
}

// Release mirrors the spotify.Release shape so queue payloads, cards and the
// worker stay symmetric across music platforms.
type Release struct {
	SourceID        string  `json:"source_id"`
	AlbumID         string  `json:"album_id,omitempty"`
	AlbumTitle      string  `json:"album_title,omitempty"`
	AlbumTrackCount int     `json:"album_track_count,omitempty"`
	Title           string  `json:"title"`
	Artists         string  `json:"artists"`
	ReleaseDate     string  `json:"release_date,omitempty"`
	CoverURL        string  `json:"cover_url,omitempty"`
	YandexURL       string  `json:"yandex_url"`
	Tracks          []Track `json:"tracks"`
}

type apiArtist struct {
	Name string `json:"name"`
}

type apiTrack struct {
	ID         any           `json:"id"`
	Title      string        `json:"title"`
	DurationMS int           `json:"durationMs"`
	Artists    []apiArtist   `json:"artists"`
	Albums     []apiAlbumRef `json:"albums"`
	CoverURI   string        `json:"coverUri"`
}

type apiAlbumRef struct {
	ID         any          `json:"id"`
	Title      string       `json:"title"`
	Year       any          `json:"year"`
	TrackCount any          `json:"trackCount"`
	CoverURI   string       `json:"coverUri"`
	Artists    []apiArtist  `json:"artists"`
	Volumes    [][]apiTrack `json:"volumes"`
}

func normalizeSingleTrack(t apiTrack) *Release {
	track := normalizeTrack(t)
	release := &Release{
		SourceID:  track.SourceID,
		Title:     track.Title,
		Artists:   track.Artists,
		CoverURL:  coverURL(t.CoverURI),
		YandexURL: track.YandexURL,
		Tracks:    []Track{track},
	}
	if len(t.Albums) > 0 {
		ref := t.Albums[0]
		release.AlbumID = anyToString(ref.ID)
		release.AlbumTitle = ref.Title
		release.AlbumTrackCount = anyToInt(ref.TrackCount)
		release.ReleaseDate = yearString(ref.Year)
		if release.CoverURL == "" {
			release.CoverURL = coverURL(ref.CoverURI)
		}
	}
	return release
}

func normalizeAlbum(a apiAlbumRef) *Release {
	tracks := make([]Track, 0)
	for _, volume := range a.Volumes {
		for _, t := range volume {
			tracks = append(tracks, normalizeTrack(t))
		}
	}
	id := anyToString(a.ID)
	return &Release{
		SourceID:    id,
		AlbumID:     id,
		AlbumTitle:  a.Title,
		Title:       a.Title,
		Artists:     formatArtists(a.Artists),
		ReleaseDate: yearString(a.Year),
		CoverURL:    coverURL(a.CoverURI),
		YandexURL:   "https://music.yandex.ru/album/" + id,
		Tracks:      tracks,
	}
}

func normalizeTrack(t apiTrack) Track {
	id := anyToString(t.ID)
	return Track{
		SourceID:   id,
		Title:      t.Title,
		Artists:    formatArtists(t.Artists),
		DurationMS: t.DurationMS,
		YandexURL:  "https://music.yandex.ru/track/" + id,
	}
}

func formatArtists(artists []apiArtist) string {
	names := make([]string, 0, len(artists))
	for _, a := range artists {
		if a.Name != "" {
			names = append(names, a.Name)
		}
	}
	return strings.Join(names, ", ")
}

func yearString(raw any) string {
	v := anyToInt(raw)
	if v <= 0 {
		return ""
	}
	return strconv.Itoa(v)
}

func anyToString(raw any) string {
	switch v := raw.(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func anyToInt(raw any) int {
	switch v := raw.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	default:
		return 0
	}
}
