package soundcloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const oembedAPI = "https://soundcloud.com/oembed"

var oembedHTTPClient = http.DefaultClient

type oembedResponse struct {
	Title        string `json:"title"`
	ThumbnailURL string `json:"thumbnail_url"`
}

func resolveCanonicalURL(raw string) (string, error) {
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	req, err := http.NewRequest(http.MethodHead, raw, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.Request == nil || resp.Request.URL == nil {
		return normalizeURL(raw), nil
	}
	return normalizeURL(resp.Request.URL.String()), nil
}

func fetchReleaseFromOEmbed(ctx context.Context, pageURL string) (*Release, error) {
	canonical, err := resolveCanonicalURL(pageURL)
	if err != nil {
		canonical = normalizeURL(pageURL)
	}

	reqURL := oembedAPI + "?format=json&url=" + url.QueryEscape(canonical)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := oembedHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: oembed request failed: %v", ErrNotFound, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: oembed status %d", ErrNotFound, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var payload oembedResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload.Title == "" {
		return nil, ErrNotFound
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

func parseOEmbedTitle(title string) (trackTitle, artist string) {
	const sep = " by "
	if i := strings.LastIndex(title, sep); i >= 0 {
		return strings.TrimSpace(title[:i]), strings.TrimSpace(title[i+len(sep):])
	}
	return strings.TrimSpace(title), ""
}

func releaseUsesYouTubeFallback(release *Release) bool {
	if release == nil {
		return false
	}
	for _, track := range release.Tracks {
		if track.YouTubeFallback {
			return true
		}
	}
	return false
}
