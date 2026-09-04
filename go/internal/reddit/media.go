package reddit

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const maxImageSize = 100 << 20 // 100 MB, Telegram's own upload cap is lower

// DownloadImage fetches a reddit-hosted image or gif into dir and returns
// the file path. The extension comes from the URL path, falling back to the
// Content-Type header.
func (c *Client) DownloadImage(ctx context.Context, rawURL, dir string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("reddit: image fetch status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageSize))
	if err != nil {
		return "", err
	}

	ext := imageExt(rawURL)
	if ext == "" {
		ext = extFromContentType(resp.Header.Get("Content-Type"))
	}
	if ext == "" {
		ext = ".jpg"
	}

	sum := sha1.Sum([]byte(rawURL))
	path := filepath.Join(dir, "img_"+hex.EncodeToString(sum[:8])+ext)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func imageExt(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	ext := filepath.Ext(u.Path)
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".mp4":
		return strings.ToLower(ext)
	}
	return ""
}

func extFromContentType(ct string) string {
	switch {
	case strings.Contains(ct, "image/jpeg"):
		return ".jpg"
	case strings.Contains(ct, "image/png"):
		return ".png"
	case strings.Contains(ct, "image/webp"):
		return ".webp"
	case strings.Contains(ct, "image/gif"):
		return ".gif"
	case strings.Contains(ct, "video/mp4"):
		return ".mp4"
	}
	return ""
}
