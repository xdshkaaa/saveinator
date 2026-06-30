package runtime

import (
	"context"
	"strings"
)

func (s *Store) PlatformMaxFileMB(ctx context.Context, platform string) int {
	key := platform + ".max_file_mb"
	if _, ok := Lookup(key); !ok {
		return s.cfg.SendVideoLimitMB
	}
	if platform == "youtube" {
		return s.CurrentInt(ctx, key, s.cfg.YouTubeMaxFileSizeMB)
	}
	if platform == "spotify" {
		return s.CurrentInt(ctx, key, s.cfg.SendDocumentLimitMB)
	}
	return s.CurrentInt(ctx, key, s.cfg.SendVideoLimitMB)
}

func (s *Store) PlatformTimeoutSec(ctx context.Context, platform string) int {
	key := platform + ".timeout_sec"
	if platform == "youtube" {
		return s.CurrentInt(ctx, key, s.cfg.YouTubeDownloadTimeoutSeconds)
	}
	if platform == "spotify" {
		return s.CurrentInt(ctx, "spotify.track_timeout_sec", s.cfg.SpotifyTrackTimeoutSeconds)
	}
	if platform == "soundcloud" {
		return s.CurrentInt(ctx, "soundcloud.track_timeout_sec", s.cfg.SoundCloudTrackTimeoutSeconds)
	}
	if platform == "pinterest" {
		return s.CurrentInt(ctx, "pinterest.timeout_sec", s.cfg.PinterestTimeoutSeconds)
	}
	return s.CurrentInt(ctx, key, s.cfg.DownloadTimeoutSeconds)
}

func (s *Store) PlatformEnabled(ctx context.Context, platform string) bool {
	key := platform + ".enabled"
	def, ok := Lookup(key)
	if !ok {
		return true
	}
	val, _ := s.CurrentValue(ctx, def)
	b, _ := val.(bool)
	return b
}

func (s *Store) CurrentBool(ctx context.Context, redisKey string, fallback bool) bool {
	def, ok := Lookup(redisKey)
	if !ok {
		return fallback
	}
	val, _ := s.CurrentValue(ctx, def)
	if b, ok := val.(bool); ok {
		return b
	}
	return fallback
}

func (s *Store) CurrentString(ctx context.Context, redisKey string, fallback string) string {
	def, ok := Lookup(redisKey)
	if !ok {
		return fallback
	}
	val, _ := s.CurrentValue(ctx, def)
	if str, ok := val.(string); ok && str != "" {
		return str
	}
	return fallback
}

func (s *Store) CurrentStringList(ctx context.Context, redisKey string, fallback []string) []string {
	def, ok := Lookup(redisKey)
	if !ok {
		return fallback
	}
	val, _ := s.CurrentValue(ctx, def)
	switch v := val.(type) {
	case []string:
		return v
	case string:
		if v == "" {
			return fallback
		}
		return splitList(v)
	default:
		return fallback
	}
}

func splitList(raw string) []string {
	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
