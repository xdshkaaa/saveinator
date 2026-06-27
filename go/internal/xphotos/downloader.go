package xphotos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	statusIDRe = regexp.MustCompile(`/status/(\d+)`)

	ErrNotFound  = errors.New("x photos not found")
	ErrDownload  = errors.New("x photo download failed")
)

const (
	fxTwitterAPI = "https://api.fxtwitter.com/status"
	vxTwitterAPI = "https://api.vxtwitter.com/status"
	userAgent    = "Mozilla/5.0 (compatible; Saveinator/1.0)"
)

type Result struct {
	Title string
	ID    string
}

func ExtractStatusID(url string) string {
	if match := statusIDRe.FindStringSubmatch(url); len(match) > 1 {
		return match[1]
	}
	return ""
}

func DownloadPhotos(ctx context.Context, url, outputDir, statusID string, maxItems int) (*Result, []string, error) {
	sid := statusID
	if sid == "" {
		sid = ExtractStatusID(url)
	}
	if sid == "" {
		return nil, nil, fmt.Errorf("%w: cannot extract status id", ErrNotFound)
	}

	title, photoURLs, err := fetchPhotoURLs(ctx, sid)
	if err != nil {
		return nil, nil, err
	}
	if maxItems > 0 && len(photoURLs) > maxItems {
		photoURLs = photoURLs[:maxItems]
	}
	if len(photoURLs) == 0 {
		return nil, nil, fmt.Errorf("%w: empty photo list for %s", ErrNotFound, sid)
	}

	var paths []string
	for i, photoURL := range photoURLs {
		ext := guessExtension(photoURL)
		path := filepath.Join(outputDir, fmt.Sprintf("photo_%d%s", i+1, ext))
		if err := downloadImage(ctx, photoURL, path); err != nil {
			continue
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return nil, nil, fmt.Errorf("%w: failed to download photos for %s", ErrDownload, sid)
	}
	return &Result{Title: title, ID: sid}, paths, nil
}

func fetchPhotoURLs(ctx context.Context, statusID string) (string, []string, error) {
	var errorsList []string
	for _, api := range []struct {
		base   string
		parser func([]byte) (string, []string, error)
	}{
		{fxTwitterAPI, parseFxTwitter},
		{vxTwitterAPI, parseVxTwitter},
	} {
		title, urls, err := fetchFromAPI(ctx, api.base, statusID, api.parser)
		if err != nil {
			errorsList = append(errorsList, fmt.Sprintf("%s: %v", api.base, err))
			continue
		}
		if len(urls) > 0 {
			return title, urls, nil
		}
		errorsList = append(errorsList, fmt.Sprintf("%s: empty photo list", api.base))
	}
	if len(errorsList) > 0 {
		return "", nil, fmt.Errorf("%w: %s", ErrNotFound, strings.Join(errorsList, "; "))
	}
	return "", nil, ErrNotFound
}

func fetchFromAPI(ctx context.Context, base, statusID string, parser func([]byte) (string, []string, error)) (string, []string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/"+statusID, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}
	return parser(body)
}

func parseFxTwitter(body []byte) (string, []string, error) {
	var data struct {
		Code    *int   `json:"code"`
		Message string `json:"message"`
		Tweet   struct {
			Text  string `json:"text"`
			Media struct {
				Photos []struct {
					URL string `json:"url"`
				} `json:"photos"`
				All []struct {
					Type string `json:"type"`
					URL  string `json:"url"`
				} `json:"all"`
			} `json:"media"`
		} `json:"tweet"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", nil, err
	}
	if data.Code != nil && *data.Code != 200 {
		return "", nil, fmt.Errorf("%s", data.Message)
	}
	var urls []string
	for _, photo := range data.Tweet.Media.Photos {
		if photo.URL != "" {
			urls = append(urls, photo.URL)
		}
	}
	if len(urls) == 0 {
		for _, item := range data.Tweet.Media.All {
			if item.Type == "photo" && item.URL != "" {
				urls = append(urls, item.URL)
			}
		}
	}
	title := strings.TrimSpace(data.Tweet.Text)
	if title == "" {
		title = "x-post"
	}
	return title, urls, nil
}

func parseVxTwitter(body []byte) (string, []string, error) {
	var data struct {
		Text          string   `json:"text"`
		MediaURLs     []string `json:"mediaURLs"`
		MediaExtended []struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"media_extended"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", nil, err
	}
	urls := append([]string{}, data.MediaURLs...)
	for _, item := range data.MediaExtended {
		if item.Type == "image" && item.URL != "" && !contains(urls, item.URL) {
			urls = append(urls, item.URL)
		}
	}
	title := strings.TrimSpace(data.Text)
	if title == "" {
		title = "x-post"
	}
	return title, urls, nil
}

func downloadImage(ctx context.Context, url, outputPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	info, err := os.Stat(outputPath)
	if err != nil || info.Size() == 0 {
		return errors.New("empty file")
	}
	return nil
}

func guessExtension(url string) string {
	path := url
	if idx := strings.Index(url, "?"); idx >= 0 {
		path = url[:idx]
	}
	path = strings.ToLower(path)
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
		if strings.HasSuffix(path, ext) {
			return ext
		}
	}
	return ".jpg"
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func IsNoVideoError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no video could be found")
}
