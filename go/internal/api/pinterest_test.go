package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"saveinator/internal/config"
	"saveinator/internal/redisx"
)

const testInternalToken = "test-internal-token"

func testRedisClient(t *testing.T) *redisx.Client {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return redisx.NewWithRedis(rdb)
}

func newTestHandler(t *testing.T, cfg *config.Settings) *PinterestHandler {
	t.Helper()
	if cfg.InternalAPIToken == "" {
		cfg.InternalAPIToken = testInternalToken
	}
	if cfg.InternalAPIRatePerMinute == 0 {
		cfg.InternalAPIRatePerMinute = 100
	}
	return NewPinterestHandler(cfg, testRedisClient(t))
}

func authedRequest(method, target string, body []byte) *http.Request {
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, bytes.NewReader(body))
	}
	req.Header.Set("X-Internal-Token", testInternalToken)
	return req
}

func TestPinterestHandler_methodNotAllowed(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t, &config.Settings{PinterestEnabled: true})
	req := authedRequest(http.MethodGet, "/download/pinterest", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestPinterestHandler_missingToken(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t, &config.Settings{PinterestEnabled: true})
	req := httptest.NewRequest(http.MethodPost, "/download/pinterest", bytes.NewReader([]byte(`{"url":"https://pin.it/abc","downloadImages":true}`)))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestPinterestHandler_wrongToken(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t, &config.Settings{PinterestEnabled: true})
	req := httptest.NewRequest(http.MethodPost, "/download/pinterest", bytes.NewReader([]byte(`{"url":"https://pin.it/abc","downloadImages":true}`)))
	req.Header.Set("X-Internal-Token", "wrong-token")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestPinterestHandler_rateLimited(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t, &config.Settings{PinterestEnabled: true, InternalAPIRatePerMinute: 1})

	req1 := authedRequest(http.MethodPost, "/download/pinterest", []byte(`{"url":"https://pin.it/abc","downloadImages":true}`))
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)

	req2 := authedRequest(http.MethodPost, "/download/pinterest", []byte(`{"url":"https://pin.it/abc","downloadImages":true}`))
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 on second request over limit", rec2.Code)
	}
}

func TestPinterestHandler_disabled(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t, &config.Settings{PinterestEnabled: false})
	req := authedRequest(http.MethodPost, "/download/pinterest", []byte(`{"url":"https://pin.it/abc","downloadImages":true}`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestPinterestHandler_invalidJSON(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t, &config.Settings{PinterestEnabled: true})
	req := authedRequest(http.MethodPost, "/download/pinterest", []byte(`not json`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestPinterestHandler_emptyURL(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t, &config.Settings{PinterestEnabled: true})
	body, _ := json.Marshal(map[string]any{
		"url":            "  ",
		"downloadImages": true,
	})
	req := authedRequest(http.MethodPost, "/download/pinterest", body)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["details"] != "url must not be empty" {
		t.Fatalf("details = %q", resp["details"])
	}
}

func TestPinterestHandler_noDownloadFlags(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t, &config.Settings{PinterestEnabled: true})
	body, _ := json.Marshal(map[string]any{
		"url": "https://www.pinterest.com/pin/123456789/",
	})
	req := authedRequest(http.MethodPost, "/download/pinterest", body)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestPinterestHandler_invalidPinterestURL(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t, &config.Settings{PinterestEnabled: true})
	body, _ := json.Marshal(map[string]any{
		"url":            "https://example.com/not-pinterest",
		"downloadImages": true,
	})
	req := authedRequest(http.MethodPost, "/download/pinterest", body)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestRegisterDownloadRoutes_disabled(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	RegisterDownloadRoutes(mux, &config.Settings{DownloadAPIEnabled: false}, testRedisClient(t))
	req := httptest.NewRequest(http.MethodPost, "/download/pinterest", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 when API disabled", rec.Code)
	}
}
