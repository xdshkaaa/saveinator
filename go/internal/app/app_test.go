package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"saveinator/internal/config"
)

func TestNew(t *testing.T) {
	t.Parallel()
	a := New(&config.Settings{})
	if a == nil || a.cfg == nil {
		t.Fatal("expected app with config")
	}
}

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	health(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("body = %q, want ok", rec.Body.String())
	}
}
