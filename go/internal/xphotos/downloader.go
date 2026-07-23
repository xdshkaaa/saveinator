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

	ErrNotFound = errors.New("x photos not found")
	ErrDownload = errors.New("x photo download failed")
	// ErrUnavailable means every source failed with a hard error (network,
	// timeout, HTTP 4xx/5xx) rather than a well-formed "no media" response.
	// Callers should surface this as a real failure, not treat it as a
	// text-only post.
	ErrUnavailable = errors.New("x api unavailable")
)

const userAgent = "Mozilla/5.0 (compatible; Saveinator/1.0)"

// fxTwitterAPI and vxTwitterAPI are vars (not consts) so tests can point
// them at an httptest server instead of the real public mirrors.
var (
	fxTwitterAPI = "https://api.fxtwitter.com/status"
	vxTwitterAPI = "https://api.vxtwitter.com/status"
)

// selfHostedBaseURL holds an optional self-hosted FxEmbed instance base URL
// (bare host, no trailing slash or path), set once at startup via Configure.
// Tried before the public fxtwitter/vxtwitter mirrors when non-empty.
var selfHostedBaseURL string

// Configure sets the optional self-hosted FxEmbed base URL (e.g.
// "https://fx.example.com"). Pass an empty string to disable it. Not
// safe for concurrent use with fetchTweet; call once at startup.
func Configure(fxEmbedBaseURL string) {
	selfHostedBaseURL = strings.TrimRight(strings.TrimSpace(fxEmbedBaseURL), "/")
}

type Result struct {
	Title string
	ID    string
}

type TweetMeta struct {
	Text   string
	Author string
}

// MediaItem is a single media entry resolved from FxTwitter/VxTwitter.
type MediaItem struct {
	URL  string
	Type string // "photo", "video", "gif"
}

// DownloadedItem is a media item downloaded to local disk.
type DownloadedItem struct {
	Path string
	Type string
}

type parsedTweet struct {
	text    string
	author  string
	media   []MediaItem
	replyTo string // status ID of the tweet this one replies to, if any
}

func (t parsedTweet) photoURLs() []string {
	var urls []string
	for _, m := range t.media {
		if m.Type == "photo" {
			urls = append(urls, m.URL)
		}
	}
	return urls
}

func ExtractStatusID(url string) string {
	if match := statusIDRe.FindStringSubmatch(url); len(match) > 1 {
		return match[1]
	}
	return ""
}

// DownloadMedia resolves and downloads all media (photos, videos, gifs) for a
// tweet via the FxTwitter/VxTwitter APIs. Video/gif items are placed first.
func DownloadMedia(ctx context.Context, url, outputDir, statusID string, maxItems int) (*Result, []DownloadedItem, error) {
	sid := statusID
	if sid == "" {
		sid = ExtractStatusID(url)
	}
	if sid == "" {
		return nil, nil, fmt.Errorf("%w: cannot extract status id", ErrNotFound)
	}

	tweet, err := fetchTweet(ctx, sid)
	if err != nil {
		return nil, nil, err
	}
	// A reply often carries no media of its own; the media lives on the
	// tweet it replies to. Fall back to the parent tweet's media in that
	// case (one hop only, to avoid chasing an entire thread).
	if len(tweet.media) == 0 && tweet.replyTo != "" {
		if parent, err := fetchTweet(ctx, tweet.replyTo); err == nil && len(parent.media) > 0 {
			tweet.media = parent.media
		}
	}
	if len(tweet.media) == 0 {
		return nil, nil, fmt.Errorf("%w: empty media list", ErrNotFound)
	}
	title := strings.TrimSpace(tweet.text)
	if title == "" {
		title = strings.TrimSpace(tweet.author)
	}
	if title == "" {
		title = "x-post"
	}

	items := tweet.media
	if maxItems > 0 && len(items) > maxItems {
		items = items[:maxItems]
	}

	var downloaded []DownloadedItem
	for i, m := range items {
		ext := guessExtension(m.URL, m.Type)
		path := filepath.Join(outputDir, fmt.Sprintf("media_%d%s", i+1, ext))
		if err := downloadImage(ctx, m.URL, path); err != nil {
			continue
		}
		downloaded = append(downloaded, DownloadedItem{Path: path, Type: m.Type})
	}
	if len(downloaded) == 0 {
		return nil, nil, fmt.Errorf("%w: failed to download media for %s", ErrDownload, sid)
	}
	return &Result{Title: title, ID: sid}, downloaded, nil
}

func FetchTweetMeta(ctx context.Context, statusID string) (*TweetMeta, error) {
	if statusID == "" {
		return nil, fmt.Errorf("%w: empty status id", ErrNotFound)
	}
	tweet, err := fetchTweet(ctx, statusID)
	if err != nil {
		return nil, err
	}
	return &TweetMeta{Text: tweet.text, Author: tweet.author}, nil
}

func fetchTweet(ctx context.Context, statusID string) (parsedTweet, error) {
	sources := []struct {
		base   string
		parser func([]byte) (parsedTweet, error)
	}{
		{fxTwitterAPI, parseFxTwitter},
		{vxTwitterAPI, parseVxTwitter},
	}
	if selfHostedBaseURL != "" {
		sources = append([]struct {
			base   string
			parser func([]byte) (parsedTweet, error)
		}{{selfHostedBaseURL + "/status", parseFxTwitter}}, sources...)
	}

	var errorsList []string
	sawEmptyPayload := false
	for _, api := range sources {
		tweet, err := fetchFromAPI(ctx, api.base, statusID, api.parser)
		if err != nil {
			errorsList = append(errorsList, fmt.Sprintf("%s: %v", api.base, err))
			continue
		}
		if tweet.text != "" || tweet.author != "" || len(tweet.media) > 0 {
			return tweet, nil
		}
		sawEmptyPayload = true
		errorsList = append(errorsList, fmt.Sprintf("%s: empty tweet payload", api.base))
	}
	joined := strings.Join(errorsList, "; ")
	// A well-formed but empty response from at least one source means the
	// tweet genuinely has no media (deleted/private/text-only). A source
	// that only ever hard-failed (timeout, HTTP error) tells us nothing
	// about the tweet itself, so that case must not be reported the same
	// way as "no media" or callers will mislabel real outages as text-only
	// posts.
	if sawEmptyPayload {
		return parsedTweet{}, fmt.Errorf("%w: %s", ErrNotFound, joined)
	}
	if len(errorsList) > 0 {
		return parsedTweet{}, fmt.Errorf("%w: %s", ErrUnavailable, joined)
	}
	return parsedTweet{}, ErrNotFound
}

func fetchFromAPI(ctx context.Context, base, statusID string, parser func([]byte) (parsedTweet, error)) (parsedTweet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/"+statusID, nil)
	if err != nil {
		return parsedTweet{}, err
	}
	req.Header.Set("User-Agent", userAgent)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return parsedTweet{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return parsedTweet{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return parsedTweet{}, err
	}
	return parser(body)
}

func parseFxTwitter(body []byte) (parsedTweet, error) {
	var data struct {
		Code    *int   `json:"code"`
		Message string `json:"message"`
		Tweet   struct {
			Text             string `json:"text"`
			ReplyingToStatus string `json:"replying_to_status"`
			Author           struct {
				Name string `json:"name"`
			} `json:"author"`
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
		return parsedTweet{}, err
	}
	if data.Code != nil && *data.Code != 200 {
		return parsedTweet{}, fmt.Errorf("%s", data.Message)
	}
	var media []MediaItem
	for _, item := range data.Tweet.Media.All {
		if item.URL != "" {
			media = append(media, MediaItem{URL: item.URL, Type: item.Type})
		}
	}
	if len(media) == 0 {
		for _, photo := range data.Tweet.Media.Photos {
			if photo.URL != "" {
				media = append(media, MediaItem{URL: photo.URL, Type: "photo"})
			}
		}
	}
	return parsedTweet{
		text:    strings.TrimSpace(data.Tweet.Text),
		author:  strings.TrimSpace(data.Tweet.Author.Name),
		media:   media,
		replyTo: strings.TrimSpace(data.Tweet.ReplyingToStatus),
	}, nil
}

func parseVxTwitter(body []byte) (parsedTweet, error) {
	var data struct {
		Text   string `json:"text"`
		Author struct {
			Name string `json:"name"`
		} `json:"author"`
		MediaURLs     []string `json:"mediaURLs"`
		ReplyingToID  string   `json:"replyingToID"`
		MediaExtended []struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"media_extended"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return parsedTweet{}, err
	}
	var media []MediaItem
	seen := map[string]bool{}
	for _, item := range data.MediaExtended {
		if item.URL == "" || seen[item.URL] {
			continue
		}
		seen[item.URL] = true
		typ := item.Type
		if typ == "image" {
			typ = "photo"
		}
		media = append(media, MediaItem{URL: item.URL, Type: typ})
	}
	for _, url := range data.MediaURLs {
		if url == "" || seen[url] {
			continue
		}
		seen[url] = true
		media = append(media, MediaItem{URL: url, Type: "photo"})
	}
	return parsedTweet{
		text:    strings.TrimSpace(data.Text),
		author:  strings.TrimSpace(data.Author.Name),
		media:   media,
		replyTo: strings.TrimSpace(data.ReplyingToID),
	}, nil
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

func guessExtension(url, mediaType string) string {
	path := url
	if idx := strings.Index(url, "?"); idx >= 0 {
		path = url[:idx]
	}
	path = strings.ToLower(path)
	exts := []string{".jpg", ".jpeg", ".png", ".webp", ".mp4", ".mov"}
	for _, ext := range exts {
		if strings.HasSuffix(path, ext) {
			return ext
		}
	}
	if mediaType == "video" || mediaType == "gif" {
		return ".mp4"
	}
	return ".jpg"
}

func IsNoVideoError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no video could be found")
}
