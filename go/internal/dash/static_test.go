package dash

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Fonts are immutable content and may be cached aggressively; the app shell
// must revalidate on every load so a fresh deploy is picked up immediately.
func TestStaticCacheHeaders(t *testing.T) {
	h := newTestServer().Router()

	tests := []struct {
		path string
		want string
	}{
		{"/", "no-cache"},
		{"/app.js", "no-cache"},
		{"/styles.css", "no-cache"},
		{"/fonts/manrope-latin.woff2", "public, max-age=31536000, immutable"},
		{"/fonts/jetbrains-mono-cyrillic.woff2", "public, max-age=31536000, immutable"},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d, want 200", tt.path, rec.Code)
			continue
		}
		if got := rec.Header().Get("Cache-Control"); got != tt.want {
			t.Errorf("%s: Cache-Control %q, want %q", tt.path, got, tt.want)
		}
	}
}
