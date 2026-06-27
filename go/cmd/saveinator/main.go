package main

import (
	"context"
	"log/slog"
	"os"

	"saveinator/internal/app"
	"saveinator/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config error", "err", err)
		os.Exit(1)
	}

	level := slog.LevelInfo
	if cfg.LogLevel == "DEBUG" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	if err := app.New(cfg).Run(context.Background()); err != nil {
		slog.Error("application stopped", "err", err)
		os.Exit(1)
	}
}
