// Package yandexmusic provides a minimal client for the public
// api.music.yandex.net endpoints used to build download cards.
//
// The API answers 451 Unavailable For Legal Reasons for requests without
// credentials coming from non-CIS IPs; sending an OAuth token lifts that
// gate, so a token is effectively required for deployments outside RU/CIS.
package yandexmusic

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	apiBase     = "https://api.music.yandex.net"
	authScheme  = "OAuth"
	defaultSize = "400x400"
)

var (
	ErrNotFound = errors.New("yandex music resource not found")
	ErrAuth     = errors.New("yandex music auth failed")
	ErrGeo      = errors.New("yandex music unavailable in region")
)

type Client struct {
	token   string
	base    string
	timeout time.Duration
	http    *http.Client
}

func NewClient(token string, timeoutSeconds int) *Client {
	return newClient(token, timeoutSeconds, apiBase)
}

func newClient(token string, timeoutSeconds int, base string) *Client {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Client{
		token:   token,
		base:    base,
		timeout: timeout,
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *Client) Enabled() bool {
	return c.token != ""
}

// FetchRelease returns a normalized release. With a non-empty trackID only
// that track is returned (with its album reference for the "whole album"
// button); otherwise the full album is fetched and its volumes flattened.
func (c *Client) FetchRelease(albumID, trackID string) (*Release, error) {
	if trackID != "" {
		return c.fetchTrack(albumID, trackID)
	}
	return c.fetchAlbum(albumID)
}

func (c *Client) fetchTrack(linkAlbumID, trackID string) (*Release, error) {
	var payload struct {
		Result []apiTrack `json:"result"`
	}
	if err := c.apiGet("/tracks/"+trackID+"?with-children=true", &payload); err != nil {
		return nil, err
	}
	if len(payload.Result) == 0 {
		return nil, ErrNotFound
	}
	release := normalizeSingleTrack(payload.Result[0])
	if linkAlbumID != "" && release.AlbumID == "" {
		release.AlbumID = linkAlbumID
	}
	if linkAlbumID != "" && release.SourceID == "" {
		release.SourceID = linkAlbumID
	}
	return release, nil
}

func (c *Client) fetchAlbum(albumID string) (*Release, error) {
	var payload struct {
		Result apiAlbumRef `json:"result"`
	}
	if err := c.apiGet("/albums/"+albumID+"/with-tracks", &payload); err != nil {
		return nil, err
	}
	return normalizeAlbum(payload.Result), nil
}

func (c *Client) apiGet(path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", authScheme+" "+c.token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	switch resp.StatusCode {
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrAuth
	case http.StatusUnavailableForLegalReasons:
		return ErrGeo
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("yandex music api status %d", resp.StatusCode)
	}
	return json.Unmarshal(body, out)
}

func coverURL(coverURI string) string {
	if coverURI == "" {
		return ""
	}
	if !strings.HasPrefix(coverURI, "http") {
		coverURI = "https://" + strings.TrimPrefix(coverURI, "//")
	}
	return strings.Replace(coverURI, "%%", defaultSize, 1)
}
