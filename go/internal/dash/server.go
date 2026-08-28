package dash

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"saveinator/internal/db"
	"saveinator/internal/redisx"
)

//go:embed static/*
var staticFS embed.FS

type Server struct {
	store         *db.Store
	redis         *redisx.Client
	probes        []Probe
	lastCheck     time.Time
	telegramToken string
	adminIDs      []int64
	sessions      *sessionManager
}

func New(store *db.Store, redisClient *redisx.Client, probes []Probe) *Server {
	s := &Server{
		store:    store,
		redis:    redisClient,
		probes:   probes,
		sessions: newSessionManager(),
	}
	s.telegramToken = os.Getenv("DASH_TELEGRAM_TOKEN")
	if s.telegramToken == "" {
		// Fall back to the main bot token so the dashboard works with the
		// same Telegram Login Widget bot in both compose and local runs.
		s.telegramToken = os.Getenv("BOT_TOKEN")
	}
	s.adminIDs = parseAdminIDs(os.Getenv("DASH_ADMIN_IDS"))
	return s
}

// parseAdminIDs parses a comma-separated list of Telegram user ids.
func parseAdminIDs(raw string) []int64 {
	var ids []int64
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.ParseInt(part, 10, 64)
		if err == nil && n > 0 {
			ids = append(ids, n)
		}
	}
	return ids
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)

	// Auth endpoints are public (the widget callback must reach them); every
	// data endpoint requires a valid session.
	r.Get("/api/health", s.handleHealth)
	r.Get("/api/auth/status", s.handleAuthStatus)
	r.Post("/api/auth/login", s.handleAuthLogin)
	r.Post("/api/auth/logout", s.handleAuthLogout)

	r.Group(func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Get("/api/overview", s.handleOverview)
		r.Get("/api/downloads", s.handleDownloads)
		r.Get("/api/platforms", s.handlePlatforms)
		r.Get("/api/users", s.handleUsers)
		r.Get("/api/users/{id}/downloads", s.handleUserDownloads)
		r.Get("/api/services", s.handleServices)
	})

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	// The app shell itself stays public so the frontend can show the login
	// screen; all data behind /api requires a session.
	// Static files are embedded into the binary and change on every deploy;
	// tell Cloudflare/browsers to revalidate so a fresh deploy is visible
	// immediately instead of being cached for hours. Fonts are immutable
	// content under versioned URLs — they can be cached for a year.
	r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".woff2") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.FileServerFS(sub).ServeHTTP(w, r)
	}))
	return r
}
