package dash

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	activeWindow     = 30 * time.Minute
	serviceTimeout   = 3 * time.Second
	servicesCacheTTL = 10 * time.Second
)

type Probe struct {
	Name string
	URL  string
}

type ServiceStatus struct {
	Name     string `json:"name"`
	Up       bool   `json:"up"`
	Latency  int64  `json:"latency_ms"`
	LastOK   bool   `json:"last_ok"`
	LastSeen int64  `json:"last_seen"`
}

type serviceCache struct {
	mu     sync.Mutex
	at     time.Time
	status []ServiceStatus
}

var cache = &serviceCache{}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	o, err := s.store.DashOverview(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	activeNow, err := s.redis.CountActiveUsers(ctx, activeWindow)
	if err != nil {
		slog.Warn("dash: count active users", "err", err)
	}
	banned, err := s.redis.BannedCount(ctx)
	if err != nil {
		slog.Warn("dash: banned count", "err", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"updated_at":           time.Now().UTC(),
		"active_now":           activeNow,
		"banned":               banned,
		"users":                o.TotalUsers,
		"new_today":            o.NewToday,
		"new_yesterday":        o.NewYesterday,
		"new_7d":               o.New7d,
		"new_30d":              o.New30d,
		"downloads_today":      o.DownloadsToday,
		"downloads_7d":         o.Downloads7d,
		"downloads_30d":        o.Downloads30d,
		"completed_30d":        o.Completed30d,
		"failed_30d":           o.Failed30d,
		"dau":                  o.DAU,
		"wau":                  o.WAU,
		"mau":                  o.MAU,
		"users_with_downloads": o.UsersWith,
		"returning_users":      o.ReturningUsers,
		"languages":            o.Languages,
		"platforms_30d":        o.Platforms30d,
		"bots":                 o.Bots,
	})
}

func (s *Server) handleDownloads(w http.ResponseWriter, r *http.Request) {
	days := intQuery(r, "days", 14)
	points, err := s.store.DownloadTimeline(r.Context(), days)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": days, "points": points})
}

func (s *Server) handlePlatforms(w http.ResponseWriter, r *http.Request) {
	days := intQuery(r, "days", 30)
	rows, err := s.store.PlatformBreakdown(r.Context(), days)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": days, "platforms": rows})
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	sortBy := r.URL.Query().Get("sort")
	if sortBy != "downloads" && sortBy != "completed" {
		sortBy = "newest"
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := intQuery(r, "limit", 200)

	rows, err := s.store.UserTable(r.Context(), sortBy, q, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": rows, "total": len(rows)})
}

func (s *Server) handleUserDownloads(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || userID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
		return
	}
	limit := intQuery(r, "limit", 200)

	rows, err := s.store.UserDownloads(r.Context(), userID, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user_id": userID, "downloads": rows, "total": len(rows)})
}

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	cache.mu.Lock()
	now := time.Now()
	if now.Sub(cache.at) < servicesCacheTTL {
		cache.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"services":   cache.status,
			"checked_at": cache.at,
		})
		return
	}
	cache.mu.Unlock()

	statuses := s.checkServices(r.Context())
	checkedAt := time.Now()

	cache.mu.Lock()
	cache.at = checkedAt
	cache.status = statuses
	cache.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"services":   statuses,
		"checked_at": checkedAt,
	})
}

// checkServices probes all configured HTTP endpoints in parallel. DB and Redis
// are checked directly through their own clients.
func (s *Server) checkServices(ctx context.Context) []ServiceStatus {
	var (
		wg       sync.WaitGroup
		statuses = make([]ServiceStatus, 0, len(s.probes)+2)
		mu       sync.Mutex
	)

	probe := func(name, url string) {
		defer wg.Done()
		client := &http.Client{Timeout: serviceTimeout}
		start := time.Now()
		resp, err := client.Get(url)
		latency := time.Since(start).Milliseconds()
		up := err == nil && resp.StatusCode >= 200 && resp.StatusCode < 400
		if resp != nil {
			_ = resp.Body.Close()
		}
		st := ServiceStatus{Name: name, Up: up, Latency: latency, LastOK: up, LastSeen: time.Now().Unix()}
		mu.Lock()
		statuses = append(statuses, st)
		mu.Unlock()
	}

	for _, p := range s.probes {
		wg.Add(1)
		go probe(p.Name, p.URL)
	}

	// DB + Redis via their own clients (no extra TCP probe needed).
	wg.Add(1)
	go func() {
		defer wg.Done()
		pctx, cancel := context.WithTimeout(ctx, serviceTimeout)
		defer cancel()
		up := s.store.Ping(pctx) == nil
		mu.Lock()
		statuses = append(statuses, ServiceStatus{Name: "db", Up: up, LastOK: up, LastSeen: time.Now().Unix()})
		mu.Unlock()
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		pctx, cancel := context.WithTimeout(ctx, serviceTimeout)
		defer cancel()
		up := s.redis.Ping(pctx) == nil
		mu.Lock()
		statuses = append(statuses, ServiceStatus{Name: "redis", Up: up, LastOK: up, LastSeen: time.Now().Unix()})
		mu.Unlock()
	}()

	wg.Wait()
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].Name < statuses[j].Name })
	return statuses
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func intQuery(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	if n <= 0 {
		return def
	}
	return n
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return math.Round(float64(a)/float64(b)*1000) / 10
}
