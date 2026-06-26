package soundcloud

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

type Client struct {
	timeoutSeconds int
	maxTracks      int
}

func NewClient(timeoutSeconds, maxTracks int) *Client {
	return &Client{timeoutSeconds: timeoutSeconds, maxTracks: maxTracks}
}

func (c *Client) FetchRelease(ctx context.Context, link *Link) (*Release, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.timeoutSeconds)*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "yt-dlp",
		"--dump-single-json", "--skip-download", "--no-warnings", "--quiet",
		link.URL,
	).Output()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotFound, err)
	}

	var info map[string]any
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, err
	}
	release := normalizeRelease(info, link.URL)
	if release.ReleaseType == "playlist" && len(release.Tracks) > c.maxTracks {
		return nil, fmt.Errorf("%w: %d tracks", ErrTooLarge, len(release.Tracks))
	}
	return release, nil
}

func normalizeRelease(info map[string]any, sourceURL string) *Release {
	entries := infoEntries(info)
	if isPlaylist(info) && len(entries) > 0 {
		tracks := make([]Track, 0, len(entries))
		for i, entry := range entries {
			tracks = append(tracks, normalizeTrack(entry, i+1))
		}
		return &Release{
			SourceID:      stringField(info, "id"),
			Title:         stringField(info, "title"),
			Artist:        artist(info),
			ReleaseType:   "playlist",
			ArtworkURL:    bestThumbnail(info),
			SoundCloudURL: firstString(stringField(info, "webpage_url"), sourceURL),
			Tracks:        tracks,
		}
	}
	trackInfo := info
	if len(entries) > 0 {
		trackInfo = entries[0]
	}
	track := normalizeTrack(trackInfo, 1)
	return &Release{
		SourceID:      stringField(info, "id"),
		Title:         firstString(track.Title, stringField(info, "title")),
		Artist:        firstString(track.Artist, artist(info)),
		ReleaseType:   "track",
		ArtworkURL:    firstString(bestThumbnail(info), track.ArtworkURL),
		SoundCloudURL: firstString(stringField(info, "webpage_url"), sourceURL),
		Tracks:        []Track{track},
	}
}

func normalizeTrack(info map[string]any, num int) Track {
	return Track{
		SourceID:      stringField(info, "id"),
		Title:         stringField(info, "title"),
		Artist:        artist(info),
		DurationMS:    int(durationMS(info["duration"])),
		SoundCloudURL: stringField(info, "webpage_url"),
		ArtworkURL:    bestThumbnail(info),
		Genre:         genre(info),
		TrackNumber:   num,
	}
}

func isPlaylist(info map[string]any) bool {
	if t, _ := info["_type"].(string); t == "playlist" || t == "multi_video" {
		return true
	}
	entries := infoEntries(info)
	return len(entries) > 1
}

func infoEntries(info map[string]any) []map[string]any {
	raw, ok := info["entries"].([]any)
	if !ok {
		return nil
	}
	var out []map[string]any
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok && m != nil {
			out = append(out, m)
		}
	}
	return out
}

func artist(info map[string]any) string {
	return firstString(
		stringField(info, "uploader"),
		stringField(info, "artist"),
		stringField(info, "creator"),
	)
}

func genre(info map[string]any) string {
	if g := stringField(info, "genre"); g != "" {
		return g
	}
	if tags, ok := info["tags"].([]any); ok && len(tags) > 0 {
		return fmt.Sprint(tags[0])
	}
	return ""
}

func bestThumbnail(info map[string]any) string {
	if t := stringField(info, "thumbnail"); t != "" {
		return t
	}
	if thumbs, ok := info["thumbnails"].([]any); ok && len(thumbs) > 0 {
		if m, ok := thumbs[len(thumbs)-1].(map[string]any); ok {
			return stringField(m, "url")
		}
	}
	return ""
}

func durationMS(raw any) float64 {
	switch v := raw.(type) {
	case float64:
		return v * 1000
	case int:
		return float64(v) * 1000
	default:
		return 0
	}
}

func stringField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func firstString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
