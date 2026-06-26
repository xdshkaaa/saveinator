package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Settings struct {
	BotToken           string
	DatabaseURL        string
	RedisURL           string
	UsePolling         bool
	WebhookHost        string
	WebhookPath        string
	WebhookPort        int
	WebhookListen      string
	WebhookSecretToken string
	LogLevel           string

	RateLimitUserPerMinute int
	RateLimitChatPerMinute int

	DownloadTimeoutSeconds int
	SendVideoLimitMB       int
	SendDocumentLimitMB    int
	YouTubeMaxFileSizeMB   int

	TikTokCookiesPath    string
	InstagramCookiesPath string

	PinterestEnabled bool

	MetricsEnabled bool
	MetricsHost    string
	MetricsPort    int

	AdminTelegramID int64

	Mode string // bot, worker, all
}

func Load() (*Settings, error) {
	loadDotEnv()

	s := &Settings{
		BotToken:               os.Getenv("BOT_TOKEN"),
		DatabaseURL:            env("DATABASE_URL", "postgres://saveinator:saveinator@localhost:5432/saveinator?sslmode=disable"),
		RedisURL:               env("REDIS_URL", "redis://localhost:6379/0"),
		UsePolling:             envBool("USE_POLLING", true),
		WebhookHost:            env("WEBHOOK_HOST", "https://saveinator-hooks.xdshka.party"),
		WebhookPath:            env("WEBHOOK_PATH", "/webhook"),
		WebhookPort:            envInt("WEBHOOK_PORT", 8000),
		WebhookListen:          env("WEBHOOK_LISTEN", "0.0.0.0"),
		WebhookSecretToken:     os.Getenv("WEBHOOK_SECRET_TOKEN"),
		LogLevel:               env("LOG_LEVEL", "INFO"),
		RateLimitUserPerMinute: envInt("RATE_LIMIT_USER_PER_MINUTE", 5),
		RateLimitChatPerMinute: envInt("RATE_LIMIT_CHAT_PER_MINUTE", 20),
		DownloadTimeoutSeconds: envInt("DOWNLOAD_TIMEOUT_SECONDS", 60),
		SendVideoLimitMB:       envInt("SEND_VIDEO_LIMIT_MB", 50),
		SendDocumentLimitMB:    envInt("SEND_DOCUMENT_LIMIT_MB", 1999),
		YouTubeMaxFileSizeMB:   envInt("YOUTUBE_MAX_FILE_SIZE_MB", 1999),
		TikTokCookiesPath:      env("TIKTOK_COOKIES_PATH", "/secrets/tiktok_cookies.txt"),
		InstagramCookiesPath:   env("INSTAGRAM_COOKIES_PATH", "/secrets/instagram_cookies.txt"),
		PinterestEnabled:       envBool("PINTEREST_ENABLED", true),
		MetricsEnabled:         envBool("METRICS_ENABLED", true),
		MetricsHost:            env("METRICS_HOST", "0.0.0.0"),
		MetricsPort:            envInt("METRICS_PORT", 9101),
		AdminTelegramID:        envInt64("ADMIN_TELEGRAM_ID", 0),
		Mode:                   strings.ToLower(env("SAVEINATOR_MODE", "all")),
	}

	if s.BotToken == "" {
		return nil, fmt.Errorf("BOT_TOKEN is required")
	}

	s.DatabaseURL = normalizePostgresURL(s.DatabaseURL)
	return s, nil
}

func loadDotEnv() {
	for _, path := range []string{".env", "../.env"} {
		if _, err := os.Stat(path); err == nil {
			_ = godotenv.Load(path)
			return
		}
	}
}

func normalizePostgresURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "postgresql+asyncpg://") {
		return "postgres://" + strings.TrimPrefix(raw, "postgresql+asyncpg://")
	}
	if strings.HasPrefix(raw, "postgresql://") {
		return "postgres://" + strings.TrimPrefix(raw, "postgresql://")
	}
	return raw
}

func (s *Settings) WebhookURL() string {
	path := s.WebhookPath
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimRight(s.WebhookHost, "/") + path
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envInt64(key string, fallback int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}
