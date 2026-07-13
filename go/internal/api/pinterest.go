package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"hash/fnv"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"saveinator/internal/config"
	"saveinator/internal/metrics"
	"saveinator/internal/pinterest"
	"saveinator/internal/redisx"
)

type PinterestHandler struct {
	cfg   *config.Settings
	redis *redisx.Client
}

func NewPinterestHandler(cfg *config.Settings, redisClient *redisx.Client) *PinterestHandler {
	return &PinterestHandler{cfg: cfg, redis: redisClient}
}

type pinterestRequest struct {
	URL            string `json:"url"`
	Limit          int    `json:"limit"`
	DownloadVideos bool   `json:"downloadVideos"`
	DownloadImages bool   `json:"downloadImages"`
}

func (h *PinterestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if allowed, err := h.redis.AllowRateLimit(r.Context(), "internal_api_ip", clientIPHash(r), h.cfg.InternalAPIRatePerMinute, time.Minute); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal error"})
		return
	} else if !allowed {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "Rate limit exceeded"})
		return
	}

	if !validInternalToken(h.cfg.InternalAPIToken, r.Header.Get("X-Internal-Token")) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	if !h.cfg.PinterestEnabled {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Pinterest downloads are disabled"})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}

	var req pinterestRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}
	req.URL = strings.TrimSpace(req.URL)
	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request", "details": "url must not be empty"})
		return
	}
	if req.Limit <= 0 {
		req.Limit = 10
	}
	if req.Limit > 50 {
		req.Limit = 50
	}
	if !req.DownloadImages && !req.DownloadVideos {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "At least one of downloadImages or downloadVideos must be true"})
		return
	}
	if _, err := pinterest.ParseURL(req.URL); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid or unsupported Pinterest URL"})
		return
	}

	taskDir, err := os.MkdirTemp("", "saveinator-pin-api-*")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Pinterest download failed"})
		return
	}
	defer os.RemoveAll(taskDir)

	timeout := time.Duration(h.cfg.PinterestTimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	client := pinterest.NewClient(h.cfg.PinterestCookiesPath, h.cfg.PinterestTimeoutSeconds)
	result, err := client.Download(ctx, req.URL, taskDir, req.Limit, req.DownloadImages, req.DownloadVideos)
	if errors.Is(err, context.DeadlineExceeded) {
		writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "Download exceeded timeout"})
		return
	}
	if errors.Is(err, pinterest.ErrNoMedia) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "private") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		slog.Warn("pinterest api download failed", "err", err)
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result.ToJSON())
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func validInternalToken(want, got string) bool {
	if want == "" || got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

// clientIPHash maps the request's remote IP to an int64 so it can reuse the
// existing AllowRateLimit(scope, id) limiter, which is keyed by int64 id.
func clientIPHash(r *http.Request) int64 {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(host))
	return int64(h.Sum64())
}

func RegisterDownloadRoutes(mux *http.ServeMux, cfg *config.Settings, redisClient *redisx.Client) {
	if !cfg.DownloadAPIEnabled {
		return
	}
	mux.Handle("/download/pinterest", metrics.HTTPMiddleware("/download/pinterest", NewPinterestHandler(cfg, redisClient)))
}
