package worker

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"saveinator/internal/config"
	"saveinator/internal/cookies"
	"saveinator/internal/ytdlp"
)

func StartMaintenance(ctx context.Context, cfg *config.Settings) {
	go runMaintenance(ctx, cfg)
}

func StartTempfileSweep(ctx context.Context) {
	go func() {
		sweepStaleTempfiles(time.Hour)
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sweepStaleTempfiles(time.Hour)
			}
		}
	}()
}

func runMaintenance(ctx context.Context, cfg *config.Settings) {
	sweepStaleTempfiles(time.Hour)

	sweepTicker := time.NewTicker(time.Hour)
	cookieTicker := time.NewTicker(5 * time.Minute)
	defer sweepTicker.Stop()
	defer cookieTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-sweepTicker.C:
			sweepStaleTempfiles(time.Hour)
		case <-cookieTicker.C:
			refreshTikTokCookies(ctx, cfg)
		}
	}
}

func sweepStaleTempfiles(olderThan time.Duration) {
	tmp := os.TempDir()
	entries, err := os.ReadDir(tmp)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-olderThan)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "saveinator-") {
			continue
		}
		path := filepath.Join(tmp, entry.Name())
		info, err := os.Stat(path)
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		if info.IsDir() {
			if err := os.RemoveAll(path); err == nil {
				slog.Info("swept stale temp dir", "path", path)
			}
			continue
		}
		if err := os.Remove(path); err == nil {
			slog.Info("swept stale temp file", "path", path)
		}
	}
}

func refreshTikTokCookies(ctx context.Context, cfg *config.Settings) {
	if !cfg.TikTokCookiesRefreshEnabled {
		return
	}
	if strings.TrimSpace(cfg.TikTokCookiesPath) == "" && strings.TrimSpace(cfg.TikTokCookiesFromBrowser) == "" {
		return
	}
	probeURL := strings.TrimSpace(cfg.TikTokCookiesRefreshURL)
	if probeURL == "" {
		return
	}
	taskDir, err := os.MkdirTemp("", "saveinator-tiktok-refresh-*")
	if err != nil {
		return
	}
	defer os.RemoveAll(taskDir)

	timeout := time.Duration(cfg.DownloadTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = time.Minute
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	tikTokCookies := cfg.TikTokCookiesPath
	if strings.TrimSpace(cfg.TikTokCookiesFromBrowser) == "" {
		tikTokCookies = cookies.SyncFromMount(cfg.TikTokCookiesPath, cookies.TikTokWritablePath)
	}
	err = ytdlp.Download(probeCtx, probeURL, taskDir, ytdlp.Options{
		FormatID:                 "best",
		Platform:                 "tiktok",
		TikTokCookies:            tikTokCookies,
		TikTokCookiesFromBrowser: cfg.TikTokCookiesFromBrowser,
		Timeout:                  timeout,
	})
	if err != nil {
		slog.Warn("tiktok cookie refresh failed", "err", err)
		return
	}
	slog.Info("tiktok cookie refresh ok")
}
