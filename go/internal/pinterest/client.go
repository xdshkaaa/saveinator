package pinterest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	pinResourceURL = "https://www.pinterest.com/resource/PinResource/get/"
	userAgent      = "Mozilla/5.0 (Windows NT 10.0; Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

type Client struct {
	http    *http.Client
	cookies string
	timeout time.Duration
}

func NewClient(cookiesPath string, timeoutSeconds int) *Client {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		http:    &http.Client{Timeout: timeout},
		cookies: cookiesPath,
		timeout: timeout,
	}
}

func (c *Client) Download(ctx context.Context, rawURL string, outputDir string, maxItems int, downloadImages, downloadVideos bool) (*DownloadResult, error) {
	parsed, err := ParseURL(rawURL)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, err
	}

	switch parsed.URLType {
	case URLTypePin, URLTypeShort:
		item, err := c.downloadPin(ctx, parsed.URL, outputDir, downloadImages, downloadVideos)
		if err != nil {
			return nil, err
		}
		return &DownloadResult{URL: parsed.URL, URLType: parsed.URLType, Items: []MediaItem{*item}}, nil
	case URLTypeBoard:
		items, err := c.downloadBoard(ctx, parsed.URL, outputDir, maxItems, downloadImages, downloadVideos)
		if err != nil {
			return nil, err
		}
		return &DownloadResult{URL: parsed.URL, URLType: parsed.URLType, Items: items}, nil
	default:
		return nil, ErrInvalidURL
	}
}

func (c *Client) downloadPin(ctx context.Context, pinURL, outputDir string, downloadImages, downloadVideos bool) (*MediaItem, error) {
	pinID, title, mediaURL, mediaType, err := c.fetchPinMedia(ctx, pinURL)
	if err != nil {
		return nil, err
	}
	if mediaType == "video" && !downloadVideos {
		return nil, ErrNoMedia
	}
	if mediaType == "image" && !downloadImages {
		return nil, ErrNoMedia
	}

	ext := filepath.Ext(strings.Split(mediaURL, "?")[0])
	if ext == "" {
		if mediaType == "video" {
			ext = ".mp4"
		} else {
			ext = ".jpg"
		}
	}
	filePath := filepath.Join(outputDir, fmt.Sprintf("pin_%s%s", pinID, ext))
	size, err := c.downloadFile(ctx, mediaURL, filePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDownload, err)
	}
	return &MediaItem{
		Title:     title,
		MediaType: mediaType,
		FilePath:  filePath,
		FileSize:  size,
	}, nil
}

func (c *Client) fetchPinMedia(ctx context.Context, pinURL string) (pinID, title, mediaURL, mediaType string, err error) {
	resolved := pinURL
	if strings.Contains(strings.ToLower(pinURL), "pin.it/") {
		resolved, err = c.resolveShortURL(ctx, pinURL)
		if err != nil {
			return "", "", "", "", err
		}
	}
	pinID = ExtractPinID(resolved)
	if pinID == "" {
		return "", "", "", "", fmt.Errorf("%w: could not resolve pin id", ErrInvalidURL)
	}

	reqURL, err := buildPinResourceURL(pinID)
	if err != nil {
		return "", "", "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", "", "", "", err
	}
	c.applyHeaders(req, fmt.Sprintf("/pin/%s/", pinID))

	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", "", "", "", fmt.Errorf("%w: pin api status %d", ErrDownload, resp.StatusCode)
	}

	var payload struct {
		ResourceResponse struct {
			Data map[string]any `json:"data"`
		} `json:"resource_response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", "", "", err
	}
	data := payload.ResourceResponse.Data
	if data == nil {
		return "", "", "", "", ErrNoMedia
	}

	title = firstString(data["grid_title"], data["description"])
	if mediaURL, ok := extractVideoURL(data); ok && mediaURL != "" {
		return pinID, title, mediaURL, "video", nil
	}
	if mediaURL, ok := extractImageURL(data); ok && mediaURL != "" {
		return pinID, title, mediaURL, "image", nil
	}
	return "", "", "", "", ErrNoMedia
}

func (c *Client) downloadBoard(ctx context.Context, boardURL, outputDir string, maxItems int, downloadImages, downloadVideos bool) ([]MediaItem, error) {
	// Board pins are fetched via Pinterest's public board feed API.
	boardPath := strings.TrimPrefix(boardURL, "https://")
	boardPath = strings.TrimPrefix(boardPath, "http://")
	if idx := strings.Index(strings.ToLower(boardPath), "pinterest."); idx >= 0 {
		boardPath = boardPath[idx:]
		slash := strings.Index(boardPath, "/")
		if slash >= 0 {
			boardPath = boardPath[slash+1:]
		}
	}
	boardPath = strings.Trim(boardPath, "/")
	if boardPath == "" {
		return nil, ErrInvalidURL
	}

	limit := maxItems
	if limit <= 0 {
		limit = 10
	}

	feedURL := fmt.Sprintf(
		"https://www.pinterest.com/resource/BoardFeedResource/get/?source_url=/%s/&data=%s",
		boardPath,
		url.QueryEscape(fmt.Sprintf(`{"options":{"board_id":"","board_url":"/%s/","page_size":%d},"context":{}}`, boardPath, limit)),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	c.applyHeaders(req, "/"+boardPath+"/")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: board api status %d", ErrDownload, resp.StatusCode)
	}

	var payload struct {
		ResourceResponse struct {
			Data []map[string]any `json:"data"`
		} `json:"resource_response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	var items []MediaItem
	for _, pin := range payload.ResourceResponse.Data {
		if len(items) >= limit {
			break
		}
		pinID := fmt.Sprint(pin["id"])
		title := firstString(pin["grid_title"], pin["description"])
		mediaURL, mediaType, ok := "", "", false
		if u, found := extractVideoURL(pin); found {
			mediaURL, mediaType, ok = u, "video", true
		} else if u, found := extractImageURL(pin); found {
			mediaURL, mediaType, ok = u, "image", true
		}
		if !ok || mediaURL == "" {
			continue
		}
		if mediaType == "video" && !downloadVideos {
			continue
		}
		if mediaType == "image" && !downloadImages {
			continue
		}
		ext := ".jpg"
		if mediaType == "video" {
			ext = ".mp4"
		}
		filePath := filepath.Join(outputDir, fmt.Sprintf("board_%s_%s%s", pinID, mediaType, ext))
		size, err := c.downloadFile(ctx, mediaURL, filePath)
		if err != nil {
			continue
		}
		items = append(items, MediaItem{
			Title:     title,
			MediaType: mediaType,
			FilePath:  filePath,
			FileSize:  size,
		})
	}
	if len(items) == 0 {
		return nil, ErrNoMedia
	}
	return items, nil
}

func buildPinResourceURL(pinID string) (string, error) {
	data := url.QueryEscape(fmt.Sprintf(`{"options":{"id":"%s","field_set_key":"detailed"},"context":{}}`, pinID))
	return pinResourceURL + "?source_url=/pin/" + pinID + "/&data=" + data, nil
}

func (c *Client) resolveShortURL(ctx context.Context, shortURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, shortURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	return resp.Request.URL.String(), nil
}

func (c *Client) applyHeaders(req *http.Request, sourcePath string) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Pinterest-PWS-Handler", "www/pin/[id].js")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", "https://www.pinterest.com"+sourcePath)
	c.loadCookies(req)
}

func (c *Client) loadCookies(req *http.Request) {
	if c.cookies == "" {
		return
	}
	raw, err := os.ReadFile(c.cookies)
	if err != nil {
		return
	}
	lines := strings.Split(string(raw), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 7 {
			continue
		}
		req.AddCookie(&http.Cookie{Name: parts[5], Value: parts[6]})
	}
}

func (c *Client) downloadFile(ctx context.Context, mediaURL, filePath string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("status %d", resp.StatusCode)
	}

	f, err := os.Create(filePath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n, err := io.Copy(f, resp.Body)
	return n, err
}

func extractImageURL(data map[string]any) (string, bool) {
	images, ok := data["images"].(map[string]any)
	if !ok {
		return "", false
	}
	if orig, ok := images["orig"].(map[string]any); ok {
		if u, ok := orig["url"].(string); ok && u != "" {
			return u, true
		}
	}
	type sized struct {
		key string
		px  int
		url string
	}
	var candidates []sized
	for key, val := range images {
		m, ok := val.(map[string]any)
		if !ok {
			continue
		}
		u, _ := m["url"].(string)
		if u == "" {
			continue
		}
		w, _ := m["width"].(float64)
		candidates = append(candidates, sized{key: key, px: int(w), url: u})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].px > candidates[j].px })
	if len(candidates) > 0 {
		return candidates[0].url, true
	}
	return "", false
}

func extractVideoURL(data map[string]any) (string, bool) {
	videos, ok := data["videos"].(map[string]any)
	if !ok {
		return "", false
	}
	list, ok := videos["video_list"].(map[string]any)
	if !ok {
		return "", false
	}
	bestURL := ""
	bestScore := -1
	for key, val := range list {
		m, ok := val.(map[string]any)
		if !ok {
			continue
		}
		u, _ := m["url"].(string)
		if u == "" {
			continue
		}
		score := 0
		if strings.Contains(key, "720") {
			score = 720
		} else if strings.Contains(key, "540") {
			score = 540
		} else if strings.Contains(strings.ToLower(key), "hls") {
			score = 100
		} else {
			score = 480
		}
		if w, ok := m["width"].(float64); ok {
			score = int(w)
		}
		if score > bestScore {
			bestScore = score
			bestURL = u
		}
	}
	return bestURL, bestURL != ""
}

func firstString(values ...any) string {
	for _, v := range values {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
