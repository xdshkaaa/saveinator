package translate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"
)

// GoogleTranslateAPI is a var (not a const) so tests can point it at an
// httptest server instead of the real public endpoint.
var GoogleTranslateAPI = "https://translate.googleapis.com/translate_a/single"

// Google translates text to Russian via the public (keyless) Google
// Translate endpoint. A short-lived in-memory cache avoids re-translating
// the same tweet text. The client itself is safe for concurrent use.
type Google struct {
	client *http.Client
	cache  sync.Map // string -> cacheEntry
}

type cacheEntry struct {
	text    string
	expires time.Time
}

const cacheTTL = 24 * time.Hour

// NewGoogle returns a Google Translate client backed by the default HTTP
// client.
func NewGoogle() *Google {
	return &Google{client: &http.Client{Timeout: 15 * time.Second}}
}

// Text translates the given text to Russian. An empty or all-Cyrillic input
// is returned as-is without an HTTP call; on any error the original text is
// returned unchanged.
func (g *Google) Text(ctx context.Context, text string) string {
	text = strings.TrimSpace(text)
	if text == "" || looksRussian(text) {
		return text
	}
	if cached, ok := g.cache.Load(text); ok {
		entry := cached.(cacheEntry)
		if time.Now().Before(entry.expires) {
			return entry.text
		}
	}

	translated := g.fetch(ctx, text)
	g.cache.Store(text, cacheEntry{text: translated, expires: time.Now().Add(cacheTTL)})
	return translated
}

func (g *Google) fetch(ctx context.Context, text string) string {
	params := url.Values{}
	params.Set("client", "gtx")
	params.Set("sl", "auto")
	params.Set("tl", "ru")
	params.Set("dt", "t")
	params.Set("q", text)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, GoogleTranslateAPI+"?"+params.Encode(), nil)
	if err != nil {
		return text
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Saveinator/1.0)")

	resp, err := g.client.Do(req)
	if err != nil {
		return text
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return text
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return text
	}

	// Response shape: [[["translated","source",null,null,10],...],null,"en",...]
	var data []any
	if err := json.Unmarshal(body, &data); err != nil || len(data) == 0 {
		return text
	}
	segments, ok := data[0].([]any)
	if !ok {
		return text
	}
	var b strings.Builder
	for _, row := range segments {
		seg, ok := row.([]any)
		if !ok || len(seg) == 0 {
			continue
		}
		if s, ok := seg[0].(string); ok {
			b.WriteString(s)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return text
	}
	return out
}

// looksRussian reports whether the text is already mostly Cyrillic and so
// needs no translation.
func looksRussian(text string) bool {
	var cyrillic, other int
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Cyrillic, r):
			cyrillic++
		case unicode.IsLetter(r):
			other++
		}
	}
	return cyrillic > 0 && cyrillic >= other
}
