package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"saveinator/internal/config"
)

func TestPinterestHandler_methodNotAllowed(t *testing.T) {
	t.Parallel()
	h := NewPinterestHandler(&config.Settings{PinterestEnabled: true})
	req := httptest.NewRequest(http.MethodGet, "/download/pinterest", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestPinterestHandler_disabled(t *testing.T) {
	t.Parallel()
	h := NewPinterestHandler(&config.Settings{PinterestEnabled: false})
	req := httptest.NewRequest(http.MethodPost, "/download/pinterest", bytes.NewReader([]byte(`{"url":"https://pin.it/abc","downloadImages":true}`)))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestPinterestHandler_invalidJSON(t *testing.T) {
	t.Parallel()
	h := NewPinterestHandler(&config.Settings{PinterestEnabled: true})
	req := httptest.NewRequest(http.MethodPost, "/download/pinterest", bytes.NewReader([]byte(`not json`)))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestPinterestHandler_emptyURL(t *testing.T) {
	t.Parallel()
	h := NewPinterestHandler(&config.Settings{PinterestEnabled: true})
	body, _ := json.Marshal(map[string]any{
		"url":            "  ",
		"downloadImages": true,
	})
	req := httptest.NewRequest(http.MethodPost, "/download/pinterest", bytes.NewReader(body))
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
	h := NewPinterestHandler(&config.Settings{PinterestEnabled: true})
	body, _ := json.Marshal(map[string]any{
		"url": "https://www.pinterest.com/pin/123456789/",
	})
	req := httptest.NewRequest(http.MethodPost, "/download/pinterest", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestPinterestHandler_invalidPinterestURL(t *testing.T) {
	t.Parallel()
	h := NewPinterestHandler(&config.Settings{PinterestEnabled: true})
	body, _ := json.Marshal(map[string]any{
		"url":            "https://example.com/not-pinterest",
		"downloadImages": true,
	})
	req := httptest.NewRequest(http.MethodPost, "/download/pinterest", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestRegisterDownloadRoutes_disabled(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	RegisterDownloadRoutes(mux, &config.Settings{DownloadAPIEnabled: false})
	req := httptest.NewRequest(http.MethodPost, "/download/pinterest", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 when API disabled", rec.Code)
	}
}
