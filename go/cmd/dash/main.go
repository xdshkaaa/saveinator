package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"saveinator/internal/dash"
	"saveinator/internal/db"
	"saveinator/internal/redisx"
)

func main() {
	if err := run(); err != nil {
		slog.Error("dash: fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	databaseURL := envOr("DATABASE_URL", "postgres://saveinator:saveinator@localhost:5432/saveinator?sslmode=disable")
	redisURL := envOr("REDIS_URL", "redis://localhost:6379/0")
	listen := envOr("DASH_LISTEN", "127.0.0.1")
	port := envOr("DASH_PORT", "9000")

	store, err := db.Connect(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer store.Close()

	redisClient, err := redisx.Connect(redisURL)
	if err != nil {
		return fmt.Errorf("connect redis: %w", err)
	}
	defer redisClient.Close()

	probes, err := parseProbes(os.Getenv("DASH_SERVICE_PROBES"))
	if err != nil {
		return err
	}

	srv := dash.New(store, redisClient, probes)
	httpSrv := &http.Server{
		Addr:              listen + ":" + port,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("dash listening", "addr", httpSrv.Addr, "probes", len(probes))
		errCh <- httpSrv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("dash shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	}
}

// parseProbes parses "name=url;name=url" into Probe entries.
func parseProbes(raw string) ([]dash.Probe, error) {
	var probes []dash.Probe
	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, url, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(name) == "" || !strings.HasPrefix(url, "http") {
			return nil, fmt.Errorf("invalid probe %q: want name=http://...", part)
		}
		probes = append(probes, dash.Probe{Name: strings.TrimSpace(name), URL: url})
	}
	return probes, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
