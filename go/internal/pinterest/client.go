package pinterest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
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
	if ext == ".m3u8" {
		ext = ".mp4"
	}
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
	if mediaURL, mediaType, ok := pinMedia(data); ok && mediaURL != "" {
		return pinID, title, mediaURL, mediaType, nil
	}
	return "", "", "", "", ErrNoMedia
}

// pinMedia resolves a pin's best media from an API payload: video (top-level
// or story-pin), falling back to the cover image.
func pinMedia(data map[string]any) (mediaURL, mediaType string, ok bool) {
	if u, found := extractVideoURL(data); found && u != "" {
		return u, "video", true
	}
	// Idea pins ("story pins") carry video only inside story_pin_data, with
	// no top-level videos object.
	for _, list := range storyPinVideoLists(data) {
		if u, found := bestVideoURL(list); found && u != "" {
			return u, "video", true
		}
	}
	if u, found := extractImageURL(data); found && u != "" {
		return u, "image", true
	}
	return "", "", false
}

// storyPinVideoLists collects the video_list maps of a story pin: one per
// page, taken from page-level video or its blocks.
func storyPinVideoLists(data map[string]any) []map[string]any {
	spd, ok := data["story_pin_data"].(map[string]any)
	if !ok {
		return nil
	}
	pages, ok := spd["pages"].([]any)
	if !ok {
		return nil
	}
	var lists []map[string]any
	for _, pageAny := range pages {
		page, ok := pageAny.(map[string]any)
		if !ok {
			continue
		}
		if v, ok := page["video"].(map[string]any); ok {
			if list, ok := v["video_list"].(map[string]any); ok && len(list) > 0 {
				lists = append(lists, list)
			}
		}
		blocks, ok := page["blocks"].([]any)
		if !ok {
			continue
		}
		for _, blockAny := range blocks {
			block, ok := blockAny.(map[string]any)
			if !ok {
				continue
			}
			if v, ok := block["video"].(map[string]any); ok {
				if list, ok := v["video_list"].(map[string]any); ok && len(list) > 0 {
					lists = append(lists, list)
				}
			}
		}
	}
	return lists
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
		mediaURL, mediaType, ok := pinMedia(pin)
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
	if strings.Contains(strings.ToLower(mediaURL), ".m3u8") {
		return c.downloadHLS(ctx, mediaURL, filePath)
	}
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

// downloadHLS remuxes an HLS playlist into a local mp4 with ffmpeg. Pinterest
// serves idea-pin video only as HLS, and a plain GET of a playlist returns a
// text file rather than media. -c copy keeps this a remux: the segments are
// already h264/aac, so no re-encode happens on the healthy path.
func (c *Client) downloadHLS(ctx context.Context, mediaURL, filePath string) (int64, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-loglevel", "error",
		"-user_agent", userAgent, "-i", mediaURL, "-c", "copy", filePath)
	if err := cmd.Run(); err != nil {
		// Some playlists remux badly; a re-encode still yields a playable file.
		retry := exec.CommandContext(ctx, "ffmpeg", "-y", "-loglevel", "error",
			"-user_agent", userAgent, "-i", mediaURL,
			"-c:v", "libx264", "-preset", "veryfast", "-c:a", "aac", filePath)
		if retryErr := retry.Run(); retryErr != nil {
			return 0, fmt.Errorf("ffmpeg hls download: %v / %v", err, retryErr)
		}
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
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
	return bestVideoURL(list)
}

// bestVideoURL picks the highest-resolution entry from a video_list, always
// preferring direct mp4 variants over HLS playlists: an mp4 downloads with a
// plain HTTP GET, while an m3u8 needs the ffmpeg remux in downloadFile.
func bestVideoURL(list map[string]any) (string, bool) {
	bestURL, bestW := "", -1
	bestHLS, bestHLSScore := "", -1
	for _, val := range list {
		m, ok := val.(map[string]any)
		if !ok {
			continue
		}
		u, _ := m["url"].(string)
		if u == "" {
			continue
		}
		width := 0
		if w, ok := m["width"].(float64); ok {
			width = int(w)
		}
		if strings.Contains(strings.ToLower(u), ".m3u8") {
			if width > bestHLSScore {
				bestHLSScore = width
				bestHLS = u
			}
			continue
		}
		if width > bestW {
			bestW = width
			bestURL = u
		}
	}
	if bestURL != "" {
		return bestURL, true
	}
	return bestHLS, bestHLS != ""
}

func firstString(values ...any) string {
	for _, v := range values {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
