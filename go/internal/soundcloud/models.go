package soundcloud

import "errors"

var (
	ErrInvalidURL = errors.New("invalid soundcloud url")
	ErrNotFound   = errors.New("soundcloud resource not found")
	ErrTooLarge   = errors.New("soundcloud playlist too large")
)

type Track struct {
	SourceID      string `json:"source_id"`
	Title         string `json:"title"`
	Artist        string `json:"artist"`
	DurationMS    int    `json:"duration_ms"`
	SoundCloudURL string `json:"soundcloud_url"`
	ArtworkURL    string `json:"artwork_url,omitempty"`
	Genre         string `json:"genre,omitempty"`
	TrackNumber   int    `json:"track_number"`
}

type Release struct {
	SourceID      string  `json:"source_id"`
	Title         string  `json:"title"`
	Artist        string  `json:"artist"`
	ReleaseType   string  `json:"release_type"`
	ArtworkURL    string  `json:"artwork_url,omitempty"`
	SoundCloudURL string  `json:"soundcloud_url"`
	Tracks        []Track `json:"tracks"`
}
