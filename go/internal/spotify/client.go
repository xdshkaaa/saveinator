package spotify

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	tokenURL = "https://accounts.spotify.com/api/token"
	apiBase  = "https://api.spotify.com/v1"
)

type Client struct {
	clientID     string
	clientSecret string
	timeout      time.Duration
	http         *http.Client

	mu    sync.Mutex
	token string
	exp   time.Time
}

func NewClient(clientID, clientSecret string, timeoutSeconds int) *Client {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		timeout:      timeout,
		http:         &http.Client{Timeout: timeout},
	}
}

func (c *Client) Enabled() bool {
	return c.clientID != "" && c.clientSecret != ""
}

func (c *Client) FetchRelease(linkType, resourceID string) (*Release, error) {
	if linkType == "track" {
		return c.fetchTrack(resourceID)
	}
	return c.fetchAlbum(resourceID)
}

func (c *Client) fetchAlbum(albumID string) (*Release, error) {
	var album map[string]any
	if err := c.apiGet(apiBase+"/albums/"+albumID, &album); err != nil {
		return nil, err
	}
	tracks, err := c.fetchAlbumTracks(albumID)
	if err != nil {
		return nil, err
	}
	return normalizeAlbum(album, tracks), nil
}

func (c *Client) fetchAlbumTracks(albumID string) ([]map[string]any, error) {
	var all []map[string]any
	offset := 0
	for {
		var payload map[string]any
		path := fmt.Sprintf("%s/albums/%s/tracks?limit=50&offset=%d", apiBase, albumID, offset)
		if err := c.apiGet(path, &payload); err != nil {
			return nil, err
		}
		items, _ := payload["items"].([]any)
		for _, item := range items {
			if m, ok := item.(map[string]any); ok {
				all = append(all, m)
			}
		}
		if payload["next"] == nil {
			break
		}
		offset += 50
	}
	return all, nil
}

func (c *Client) fetchTrack(trackID string) (*Release, error) {
	var track map[string]any
	if err := c.apiGet(apiBase+"/tracks/"+trackID, &track); err != nil {
		return nil, err
	}
	return releaseFromTrack(track), nil
}

func (c *Client) apiGet(path string, out any) error {
	token, err := c.accessToken()
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 404 {
		return ErrNotFound
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return ErrAuth
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("spotify api status %d", resp.StatusCode)
	}
	return json.Unmarshal(body, out)
}

func (c *Client) accessToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.exp.Add(-30*time.Second)) {
		return c.token, nil
	}
	if !c.Enabled() {
		return "", ErrAuth
	}
	form := url.Values{"grant_type": {"client_credentials"}}
	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+basicAuth(c.clientID, c.clientSecret))
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.AccessToken == "" {
		return "", ErrAuth
	}
	c.token = payload.AccessToken
	c.exp = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second)
	return c.token, nil
}

func basicAuth(id, secret string) string {
	return base64.StdEncoding.EncodeToString([]byte(id + ":" + secret))
}
