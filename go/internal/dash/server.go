package dash

import (
	"embed"
	"io/fs"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"saveinator/internal/db"
	"saveinator/internal/redisx"
)

//go:embed static/*
var staticFS embed.FS

type Server struct {
	store      *db.Store
	redis      *redisx.Client
	probes     []Probe
	lastCheck  time.Time
}

func New(store *db.Store, redisClient *redisx.Client, probes []Probe) *Server {
	return &Server{store: store, redis: redisClient, probes: probes}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)

	r.Get("/api/health", s.handleHealth)
	r.Get("/api/overview", s.handleOverview)
	r.Get("/api/downloads", s.handleDownloads)
	r.Get("/api/platforms", s.handlePlatforms)
	r.Get("/api/users", s.handleUsers)
	r.Get("/api/services", s.handleServices)

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	r.Handle("/*", http.FileServerFS(sub))
	return r
}
