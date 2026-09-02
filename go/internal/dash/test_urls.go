package dash

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"saveinator/internal/db"
	"saveinator/internal/linkparser"
)

// testablePlatforms are the platforms the worker test runner can exercise.
// Music scenarios (spotify, soundcloud, yandexmusic) are chat-bound and
// deliberately excluded.
var testablePlatforms = map[string]bool{
	"youtube":   true,
	"tiktok":    true,
	"instagram": true,
	"x":         true,
	"pinterest": true,
}

// testablePlatform extracts the first link from text and checks its platform
// is one the test runner supports.
func testablePlatform(text string) (platform, url string, err error) {
	links := linkparser.ExtractURLs(text)
	if len(links) == 0 {
		return "", "", errors.New("no link found")
	}
	link := links[0]
	if !testablePlatforms[string(link.Platform)] {
		return "", "", fmt.Errorf("platform %q is not testable (supported: youtube, tiktok, instagram, x, pinterest)", link.Platform)
	}
	return string(link.Platform), link.URL, nil
}

func (s *Server) handleTestURLs(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListTestURLs(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if rows == nil {
		rows = []db.TestURLRow{}
	}
	counts := map[string]int{"pending": 0, "running": 0, "passed": 0, "failed": 0}
	for _, row := range rows {
		switch row.Status {
		case db.TestStatusPending:
			counts["pending"]++
		case db.TestStatusRunning:
			counts["running"]++
		case db.TestStatusPassed:
			counts["passed"]++
		case db.TestStatusFailed:
			counts["failed"]++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows, "counts": counts})
}

func (s *Server) handleTestURLCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body: " + err.Error()})
		return
	}
	platform, url, err := testablePlatform(req.URL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	row, err := s.store.CreateTestURL(r.Context(), url, platform)
	if errors.Is(err, db.ErrDuplicateTestURL) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "url is already on the list"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) handleTestURLRun(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.RequeueTestURLs(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requeued": n})
}

func (s *Server) handleTestURLRerun(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	n, err := s.store.RequeueTestURLs(r.Context(), &id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if n == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "row not found or not finished"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requeued": n})
}

func (s *Server) handleTestURLDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := s.store.DeleteTestURL(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
