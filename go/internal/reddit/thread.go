package reddit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Errors returned by Client.Thread; callers match on them to show a
// user-facing message instead of a generic failure.
var (
	ErrNotFound    = errors.New("reddit: thread not found or unavailable")
	ErrRateLimited = errors.New("reddit: rate limited by reddit")
)

// Media is one downloadable asset attached to the post.
// Type is one of "image" (static picture), "gif" (animated, delivered as an
// mp4) or "video" (a hosted reddit video; fetched via yt-dlp).
type Media struct {
	Type string
	URL  string
}

type Comment struct {
	Author string
	Score  int64
	Body   string
}

type Thread struct {
	ID          string
	Title       string
	Author      string
	Subreddit   string
	Selftext    string
	Score       int64
	NumComments int64
	Permalink   string
	Media       []Media
	Comments    []Comment
}

// HasText reports whether the post carries its own text content.
func (t *Thread) HasText() bool {
	return t.Selftext != ""
}

// HasMedia reports whether the post carries downloadable assets.
func (t *Thread) HasMedia() bool {
	return len(t.Media) > 0
}

type Client struct {
	client *http.Client
	// apiBase is the origin serving the public JSON API. Tests point it at
	// an httptest server; production uses www.reddit.com.
	apiBase string
	// cookieHeader is sent on every request; reddit returns 403 for the
	// public JSON endpoints without an authenticated session. Empty means
	// no cookie file is available (local dev with REDDIT_COOKIES_FROM_BROWSER).
	cookieHeader string
}

// NewClient returns a Reddit client with the given request timeout in
// seconds (falls back to 30s when the value is not sane). cookieFile points
// at a Netscape cookie file with a logged-in reddit session; may be empty.
func NewClient(timeoutSec int, cookieFile string) *Client {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	return &Client{
		client:       &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
		apiBase:      "https://www.reddit.com",
		cookieHeader: LoadCookieHeader(cookieFile),
	}
}

// The public JSON endpoint rejects default Go/undocumented user agents with
// 403s; a plain browser UA keeps it happy without any OAuth setup.
const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// Thread fetches a thread (post + top-level comments sorted by score) by its
// base36 ID. maxComments bounds the comment list; values <= 0 skip comments.
func (c *Client) Thread(ctx context.Context, threadID string, maxComments int) (*Thread, error) {
	if threadID == "" {
		return nil, ErrNotFound
	}
	if maxComments < 0 {
		maxComments = 0
	}
	params := url.Values{}
	params.Set("raw_json", "1")
	params.Set("sort", "top")
	params.Set("depth", "1")
	params.Set("limit", fmt.Sprint(maxComments))

	apiURL := c.apiBase + "/comments/" + threadID + ".json?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	if c.cookieHeader != "" {
		req.Header.Set("Cookie", c.cookieHeader)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// keep going
	case http.StatusTooManyRequests:
		return nil, ErrRateLimited
	case http.StatusNotFound, http.StatusForbidden:
		return nil, ErrNotFound
	default:
		return nil, fmt.Errorf("reddit: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}

	return parseThread(body)
}

type galleryData struct {
	Items []struct {
		MediaID string `json:"media_id"`
	} `json:"items"`
}

type galleryMetadata struct {
	Status string `json:"status"`
	E      string `json:"e"` // "Image" or "AnimatedImage"
	S      struct {
		U   string `json:"u"` // image URL
		Gif string `json:"gif"`
		Mp4 string `json:"mp4"`
	} `json:"s"`
}

// previewPayload mirrors the real reddit shape: "preview" is an object
// holding the images array, not an array itself.
type previewPayload struct {
	Images []previewImage `json:"images"`
}

type previewImage struct {
	// Source/resolutions are unused here; only animation variants matter.
	Variants struct {
		Gif *previewVariant `json:"gif"`
		Mp4 *previewVariant `json:"mp4"`
	} `json:"variants"`
}

type previewVariant struct {
	Source struct {
		URL string `json:"url"`
	} `json:"source"`
}

type postPayload struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Author      string `json:"author"`
	Subreddit   string `json:"subreddit"`
	Selftext    string `json:"selftext"`
	Score       int64  `json:"score"`
	NumComments int64  `json:"num_comments"`
	Permalink   string `json:"permalink"`
	// Media and gallery payloads (only the fields we consume).
	IsGallery     bool                       `json:"is_gallery"`
	GalleryData   *galleryData               `json:"gallery_data"`
	MediaMetadata map[string]galleryMetadata `json:"media_metadata"`
	IsVideo       bool                       `json:"is_video"`
	PostHint      string                     `json:"post_hint"`
	URL           string                     `json:"url"`
	URLDisplay    string                     `json:"url_overridden_by_dest"`
	Preview       *previewPayload            `json:"preview"`
}

// PostURL returns the canonical absolute URL of the post, used both for
// yt-dlp downloads and as the "original" link inside articles.
func (p *postPayload) PostURL() string {
	return "https://www.reddit.com" + p.Permalink
}

func parseThread(body []byte) (*Thread, error) {
	var raw []struct {
		Data struct {
			Children []struct {
				Kind string          `json:"kind"`
				Data json.RawMessage `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("reddit: decode thread json: %w", err)
	}
	if len(raw) < 2 || len(raw[0].Data.Children) == 0 {
		return nil, ErrNotFound
	}

	var post postPayload
	if err := json.Unmarshal(raw[0].Data.Children[0].Data, &post); err != nil {
		return nil, fmt.Errorf("reddit: decode post json: %w", err)
	}

	thread := &Thread{
		ID:          post.ID,
		Title:       post.Title,
		Author:      post.Author,
		Subreddit:   post.Subreddit,
		Selftext:    normalizeSelftext(post.Selftext),
		Score:       post.Score,
		NumComments: post.NumComments,
		Permalink:   post.PostURL(),
		Media:       extractMedia(&post),
	}

	for _, child := range raw[1].Data.Children {
		if child.Kind != "t1" {
			continue
		}
		var c struct {
			Author string `json:"author"`
			Score  int64  `json:"score"`
			Body   string `json:"body"`
		}
		if err := json.Unmarshal(child.Data, &c); err != nil {
			continue
		}
		body := normalizeSelftext(c.Body)
		if c.Author == "" || c.Author == "[deleted]" || body == "" ||
			body == "[deleted]" || body == "[removed]" {
			continue
		}
		thread.Comments = append(thread.Comments, Comment{Author: c.Author, Score: c.Score, Body: body})
	}

	return thread, nil
}

// extractMedia collects downloadable assets from the post payload: galleries,
// direct i.redd.it images, animated previews and hosted videos. Video files
// are not resolved here — the worker hands the post URL to yt-dlp instead.
func extractMedia(post *postPayload) []Media {
	var media []Media

	switch {
	case post.IsGallery:
		for _, item := range galleryItems(post.GalleryData, post.MediaMetadata) {
			item.URL = unescapeURL(item.URL)
			media = append(media, item)
		}
	case post.IsVideo || post.PostHint == "hosted:video":
		media = append(media, Media{Type: "video", URL: post.PostURL()})
	case isImageDest(post.URLDisplay):
		media = append(media, Media{Type: "image", URL: unescapeURL(post.URLDisplay)})
	case post.PostHint == "image" && isImageDest(post.URL):
		media = append(media, Media{Type: "image", URL: unescapeURL(post.URL)})
	case hasAnimatedPreview(post.Preview):
		m := animatedPreview(post.Preview)
		m.URL = unescapeURL(m.URL)
		media = append(media, m)
	}
	return media
}

// unescapeURL undoes HTML entity escaping that reddit still leaks into some
// media URLs despite raw_json=1 ("&amp;" → "&").
func unescapeURL(u string) string {
	return strings.ReplaceAll(u, "&amp;", "&")
}

func galleryItems(gallery *galleryData, metadata map[string]galleryMetadata) []Media {
	var out []Media
	if gallery == nil || metadata == nil {
		return out
	}
	for _, item := range gallery.Items {
		m, ok := metadata[item.MediaID]
		if !ok || m.Status != "valid" {
			continue
		}
		if m.E == "AnimatedImage" && m.S.Mp4 != "" {
			out = append(out, Media{Type: "gif", URL: m.S.Mp4})
			continue
		}
		if m.S.U != "" {
			out = append(out, Media{Type: "image", URL: m.S.U})
		}
	}
	return out
}

func hasAnimatedPreview(preview *previewPayload) bool {
	if preview == nil || len(preview.Images) == 0 {
		return false
	}
	v := preview.Images[0].Variants
	return (v.Mp4 != nil && v.Mp4.Source.URL != "") || (v.Gif != nil && v.Gif.Source.URL != "")
}

func animatedPreview(preview *previewPayload) Media {
	v := preview.Images[0].Variants
	if v.Mp4 != nil && v.Mp4.Source.URL != "" {
		return Media{Type: "gif", URL: v.Mp4.Source.URL}
	}
	return Media{Type: "gif", URL: v.Gif.Source.URL}
}

func isImageDest(u string) bool {
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp", ".gif"} {
		if strings.HasSuffix(strings.ToLower(u), ext) {
			return true
		}
	}
	return false
}

var (
	threadShortRe    = regexp.MustCompile(`(?i)redd\.it/([A-Za-z0-9]+)`)
	threadCommentsRe = regexp.MustCompile(`(?i)reddit\.com/(?:r/[\w-]+/)?comments/([A-Za-z0-9]+)`)
)

// ExtractThreadID pulls the base36 thread ID out of a full comments URL or a
// redd.it short link. Returns "" when the URL carries neither.
func ExtractThreadID(u string) string {
	for _, re := range []*regexp.Regexp{threadShortRe, threadCommentsRe} {
		if m := re.FindStringSubmatch(u); len(m) > 1 {
			return m[1]
		}
	}
	return ""
}
