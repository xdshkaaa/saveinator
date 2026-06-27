package runtime

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"saveinator/internal/config"
	"saveinator/internal/redisx"
)

type Store struct {
	redis *redisx.Client
	cfg   *config.Settings
}

func NewStore(redis *redisx.Client, cfg *config.Settings) *Store {
	return &Store{redis: redis, cfg: cfg}
}

func (s *Store) CurrentValue(ctx context.Context, def Setting) (any, error) {
	raw, ok, err := s.redis.RuntimeGet(ctx, def.RedisKey)
	if err != nil {
		return defaultFromConfig(s.cfg, def), nil
	}
	if !ok {
		return defaultFromConfig(s.cfg, def), nil
	}
	return deserialise(raw, def), nil
}

func (s *Store) CurrentInt(ctx context.Context, redisKey string, fallback int) int {
	def, ok := Lookup(redisKey)
	if !ok {
		return fallback
	}
	val, err := s.CurrentValue(ctx, def)
	if err != nil {
		return fallback
	}
	if n, ok := val.(int); ok {
		return n
	}
	return fallback
}

func (s *Store) SetValue(ctx context.Context, redisKey string, value any) error {
	def, ok := Lookup(redisKey)
	if !ok {
		return fmt.Errorf("unknown setting %s", redisKey)
	}
	return s.redis.RuntimeSet(ctx, redisKey, serialise(value, def))
}

func (s *Store) Reset(ctx context.Context, redisKey string) error {
	return s.redis.RuntimeDelete(ctx, redisKey)
}

func (s *Store) ResetAll(ctx context.Context) error {
	return s.redis.RuntimeDeleteAll(ctx)
}

func (s *Store) Validate(def Setting, raw string) error {
	raw = strings.TrimSpace(raw)
	switch def.ValueType {
	case TypeBool:
		lower := strings.ToLower(raw)
		if lower == "1" || lower == "0" || lower == "true" || lower == "false" || lower == "yes" || lower == "no" || lower == "on" || lower == "off" {
			return nil
		}
		return fmt.Errorf("expected true/false")
	case TypeEnum:
		for _, allowed := range def.Allowed {
			if raw == allowed {
				return nil
			}
		}
		return fmt.Errorf("allowed: %s", strings.Join(def.Allowed, ", "))
	case TypeList:
		for _, part := range splitList(raw) {
			if !allowedValue(def.Allowed, part) {
				return fmt.Errorf("invalid value %q", part)
			}
		}
		if raw == "" {
			return fmt.Errorf("list cannot be empty")
		}
		return nil
	case TypeInt:
		n, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("expected positive integer")
		}
		if def.MinValue > 0 && n < def.MinValue {
			return fmt.Errorf("min %d", def.MinValue)
		}
		if def.MaxValue > 0 && n > def.MaxValue {
			return fmt.Errorf("max %d", def.MaxValue)
		}
		return nil
	default:
		return fmt.Errorf("unsupported type")
	}
}

func (s *Store) Parse(def Setting, raw string) any {
	raw = strings.TrimSpace(raw)
	switch def.ValueType {
	case TypeBool:
		lower := strings.ToLower(raw)
		return lower == "1" || lower == "true" || lower == "yes" || lower == "on"
	case TypeEnum:
		return raw
	case TypeList:
		return strings.Join(splitList(raw), ",")
	default:
		n, _ := strconv.Atoi(raw)
		return n
	}
}

func FormatValue(value any, def Setting, lang string) string {
	switch def.ValueType {
	case TypeBool:
		if b, ok := value.(bool); ok && b {
			if lang == "ru" {
				return "да"
			}
			return "yes"
		}
		if lang == "ru" {
			return "нет"
		}
		return "no"
	case TypeEnum, TypeList:
		return fmt.Sprintf("%v", value)
	default:
		if def.Unit != "" {
			return fmt.Sprintf("%v %s", value, def.Unit)
		}
		return fmt.Sprintf("%v", value)
	}
}

func serialise(value any, def Setting) string {
	switch def.ValueType {
	case TypeBool:
		if b, ok := value.(bool); ok && b {
			return "1"
		}
		return "0"
	case TypeEnum, TypeList:
		return fmt.Sprintf("%v", value)
	default:
		return fmt.Sprintf("%v", value)
	}
}

func deserialise(raw string, def Setting) any {
	switch def.ValueType {
	case TypeBool:
		return raw == "1" || strings.EqualFold(raw, "true")
	case TypeEnum, TypeList:
		return raw
	default:
		n, _ := strconv.Atoi(raw)
		return n
	}
}

func allowedValue(allowed []string, value string) bool {
	for _, a := range allowed {
		if a == value {
			return true
		}
	}
	return false
}

func defaultFromConfig(cfg *config.Settings, def Setting) any {
	if cfg != nil {
		switch def.ConfigField {
		case "SendDocumentLimitMB":
			return cfg.SendDocumentLimitMB
		case "TelegramUploadLimitMB":
			return cfg.TelegramUploadLimitMB
		case "DownloadTimeoutSeconds":
			return cfg.DownloadTimeoutSeconds
		case "BroadcastDelayMS":
			return cfg.BroadcastDelayMS
		case "BroadcastBatchSize":
			return cfg.BroadcastBatchSize
		case "YouTubeMaxFileSizeMB":
			return cfg.YouTubeMaxFileSizeMB
		case "YouTubeDownloadTimeoutSeconds":
			return cfg.YouTubeDownloadTimeoutSeconds
		case "SendVideoLimitMB":
			return cfg.SendVideoLimitMB
		case "SpotifyEnabled":
			return cfg.SpotifyEnabled
		case "SpotifyDownloadEnabled":
			return cfg.SpotifyDownloadEnabled
		case "SpotifyTrackTimeoutSeconds":
			return cfg.SpotifyTrackTimeoutSeconds
		case "SpotifyAPITimeoutSeconds":
			return cfg.SpotifyAPITimeoutSeconds
		case "SpotifyLockMaxTracks":
			return cfg.SpotifyLockMaxTracks
		case "SpotifyDownloadConcurrency":
			return cfg.SpotifyDownloadConcurrency
		case "SoundCloudEnabled":
			return cfg.SoundCloudEnabled
		case "SoundCloudDownloadEnabled":
			return cfg.SoundCloudDownloadEnabled
		case "SoundCloudTrackTimeoutSeconds":
			return cfg.SoundCloudTrackTimeoutSeconds
		case "SoundCloudMaxTracks":
			return cfg.SoundCloudMaxTracks
		case "SoundCloudDownloadConcurrency":
			return cfg.SoundCloudDownloadConcurrency
		case "PinterestEnabled":
			return cfg.PinterestEnabled
		case "PinterestTimeoutSeconds":
			return cfg.PinterestTimeoutSeconds
		case "PinterestMaxItems":
			return cfg.PinterestMaxItems
		case "PinterestDownloadImages":
			return cfg.PinterestDownloadImages
		case "PinterestDownloadVideos":
			return cfg.PinterestDownloadVideos
		case "TikTokCarouselMaxItems":
			return cfg.TikTokCarouselMaxItems
		case "TikTokCarouselAudioEnabled":
			return cfg.TikTokCarouselAudioEnabled
		case "SoundCloudDLOutputFormat":
			return cfg.SoundCloudDLOutputFormat
		}
	}
	if def.ValueType == TypeEnum || def.ValueType == TypeList {
		if def.DefaultStr != "" {
			return def.DefaultStr
		}
	}
	if def.ValueType == TypeBool {
		return def.DefaultBool
	}
	if def.DefaultInt != 0 {
		return def.DefaultInt
	}
	if def.ValueType == TypeBool {
		return false
	}
	return 0
}
