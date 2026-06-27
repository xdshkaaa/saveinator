package instagram

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"saveinator/internal/cookies"
)

var (
	ErrNotFound     = errors.New("instagram photos not found")
	ErrDownload     = errors.New("instagram photo download failed")
	ErrAuthRequired = errors.New("instagram auth required")
)

const userAgent = "Mozilla/5.0 (compatible; Saveinator/1.0)"

var mediaURLTemplate = "https://www.instagram.com/p/%s/media/?size=l"

func mediaURL(shortcode string, index int) string {
	url := fmt.Sprintf(mediaURLTemplate, shortcode)
	if index > 1 {
		url += fmt.Sprintf("&index=%d", index)
	}
	return url
}

type PhotoResult struct {
	Title     string
	Shortcode string
}

type PhotoClient struct {
	cookiesPath string
	http        *http.Client
	mediaURL    func(shortcode string, index int) string
}

func NewPhotoClient(cookiesPath string) *PhotoClient {
	path := strings.TrimSpace(cookiesPath)
	if synced := cookies.SyncFromMount(path, cookies.InstagramWritablePath); synced != "" {
		path = synced
	}
	return &PhotoClient{
		cookiesPath: path,
		http:        &http.Client{Timeout: 60 * time.Second},
		mediaURL:    mediaURL,
	}
}

func (c *PhotoClient) DownloadPhotos(ctx context.Context, url, outputDir string, maxItems int) (*PhotoResult, []string, error) {
	shortcode := ExtractShortcode(url)
	if shortcode == "" {
		return nil, nil, fmt.Errorf("%w: missing shortcode", ErrNotFound)
	}
	if maxItems < 1 {
		maxItems = 1
	}

	var paths []string
	seen := map[string]struct{}{}
	for i := 1; i <= maxItems; i++ {
		path := filepath.Join(outputDir, fmt.Sprintf("photo_%d.jpg", i))
		hash, err := c.downloadMedia(ctx, c.mediaURL(shortcode, i), path)
		if err != nil {
			if i == 1 {
				return nil, nil, err
			}
			break
		}
		if _, ok := seen[hash]; ok {
			_ = os.Remove(path)
			break
		}
		seen[hash] = struct{}{}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return nil, nil, fmt.Errorf("%w: no photos for %s", ErrNotFound, shortcode)
	}
	return &PhotoResult{Shortcode: shortcode}, paths, nil
}

func (c *PhotoClient) downloadMedia(ctx context.Context, mediaURL, outputPath string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	c.loadCookies(req)

	client := *c.http
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		c.loadCookies(req)
		req.Header.Set("User-Agent", userAgent)
		return nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrDownload, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", ErrAuthRequired
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("%w: HTTP %d", ErrDownload, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if len(body) == 0 {
		return "", fmt.Errorf("%w: empty response", ErrDownload)
	}
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/html") {
		if strings.Contains(strings.ToLower(string(body)), "login") {
			return "", ErrAuthRequired
		}
		return "", fmt.Errorf("%w: unexpected HTML response", ErrDownload)
	}

	if err := os.WriteFile(outputPath, body, 0o644); err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func (c *PhotoClient) loadCookies(req *http.Request) {
	if c.cookiesPath == "" {
		return
	}
	raw, err := os.ReadFile(c.cookiesPath)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(raw), "\n") {
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

func UserFacingErrorKey(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrAuthRequired) {
		return "instagram.auth_required"
	}
	return "instagram.download_failed"
}
