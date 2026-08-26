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
	BotUsername        string
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
	SpamDedupWindowSeconds int

	DownloadTimeoutSeconds int
	SendVideoLimitMB       int
	SendDocumentLimitMB    int
	TelegramUploadLimitMB  int
	YouTubeMaxFileSizeMB          int
	YouTubeDownloadTimeoutSeconds int
	YouTubeEnabled                bool
	YouTubeTranscodeEnabled       bool
	YouTubeCompressLongEnabled    bool
	YouTubeCompressMinDurationSec int

	BroadcastDelayMS   int
	BroadcastBatchSize  int

	TikTokCookiesPath              string
	TikTokCookiesFromBrowser       string
	TikTokCookiesRefreshEnabled    bool
	TikTokCookiesRefreshURL        string
	TikTokReferer                  string

	PinterestEnabled         bool
	PinterestTimeoutSeconds  int
	PinterestMaxItems        int
	PinterestDownloadImages  bool
	PinterestDownloadVideos  bool
	PinterestCookiesPath     string

	InstagramEnabled            bool
	InstagramTimeoutSeconds     int
	InstagramMaxFileMB          int
	InstagramCookiesPath        string
	InstagramCookiesFromBrowser string

	TikTokCarouselMaxItems    int
	TikTokCarouselAudioEnabled bool

	SpotifyEnabled              bool
	SpotifyClientID             string
	SpotifyClientSecret         string
	SpotifyAPITimeoutSeconds    int
	SpotifyDownloadEnabled      bool
	SpotifyTrackTimeoutSeconds  int
	SpotifyDLOutputFormat       string
	SpotifyLockMaxTracks        int
	SpotifyDownloadConcurrency  int

	SoundCloudEnabled             bool
	SoundCloudDownloadEnabled     bool
	SoundCloudTrackTimeoutSeconds int
	SoundCloudMaxTracks           int
	SoundCloudDLOutputFormat      string
	SoundCloudDownloadConcurrency int

	YandexMusicEnabled              bool
	YandexMusicAccessToken          string
	YandexMusicAPITimeoutSeconds    int
	YandexMusicDownloadEnabled      bool
	YandexMusicTrackTimeoutSeconds  int
	YandexMusicDLOutputFormat       string
	YandexMusicLockMaxTracks        int
	YandexMusicDownloadConcurrency  int

	MetricsEnabled bool
	MetricsHost    string
	MetricsPort    int
	WorkerMetricsPort int

	AdminTelegramID int64

	DownloadAPIEnabled bool
	InternalAPIToken   string
	InternalAPIRatePerMinute int

	Mode string // bot, worker, all
}

func Load() (*Settings, error) {
	loadDotEnv()

	s := &Settings{
		BotToken:               os.Getenv("BOT_TOKEN"),
		BotUsername:            env("BOT_USERNAME", ""),
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
		SpamDedupWindowSeconds: envInt("SPAM_DEDUP_WINDOW_SECONDS", 300),
		DownloadTimeoutSeconds: envInt("DOWNLOAD_TIMEOUT_SECONDS", 60),
		SendVideoLimitMB:       envInt("SEND_VIDEO_LIMIT_MB", 50),
		SendDocumentLimitMB:    envInt("SEND_DOCUMENT_LIMIT_MB", 1999),
		TelegramUploadLimitMB:  envInt("TELEGRAM_BOT_UPLOAD_LIMIT_MB", 50),
		YouTubeMaxFileSizeMB:          envInt("YOUTUBE_MAX_FILE_SIZE_MB", 1999),
		YouTubeDownloadTimeoutSeconds: envInt("YOUTUBE_DOWNLOAD_TIMEOUT_SECONDS", 600),
		YouTubeEnabled:                envBool("YOUTUBE_ENABLED", true),
		YouTubeTranscodeEnabled:       envBool("YOUTUBE_TRANSCODE_ENABLED", true),
		YouTubeCompressLongEnabled:    envBool("YOUTUBE_COMPRESS_LONG_ENABLED", true),
		YouTubeCompressMinDurationSec: envInt("YOUTUBE_COMPRESS_MIN_DURATION_SEC", 600),
		BroadcastDelayMS:    envInt("BROADCAST_DELAY_MS", 50),
		BroadcastBatchSize:  envInt("BROADCAST_BATCH_SIZE", 20),
		TikTokCookiesPath:              env("TIKTOK_COOKIES_PATH", "/secrets/tiktok_cookies.txt"),
		TikTokCookiesFromBrowser:       env("TIKTOK_COOKIES_FROM_BROWSER", ""),
		TikTokCookiesRefreshEnabled:    envBool("TIKTOK_COOKIES_REFRESH_ENABLED", true),
		TikTokCookiesRefreshURL:        env("TIKTOK_COOKIES_REFRESH_URL", "https://vt.tiktok.com/ZSCFGyN3g/"),
		TikTokReferer:                  env("TIKTOK_REFERER", "https://www.tiktok.com/"),
		PinterestEnabled:         envBool("PINTEREST_ENABLED", true),
		PinterestTimeoutSeconds:  envInt("PINTEREST_TIMEOUT_SECONDS", 30),
		PinterestMaxItems:        envInt("PINTEREST_MAX_ITEMS", 10),
		PinterestDownloadImages:  envBool("PINTEREST_DOWNLOAD_IMAGES", true),
		PinterestDownloadVideos:  envBool("PINTEREST_DOWNLOAD_VIDEOS", true),
		PinterestCookiesPath:     env("PINTEREST_COOKIES_PATH", ""),
		InstagramEnabled:            envBool("INSTAGRAM_ENABLED", true),
		InstagramTimeoutSeconds:     envInt("INSTAGRAM_DOWNLOAD_TIMEOUT_SECONDS", 60),
		InstagramMaxFileMB:          envInt("INSTAGRAM_MAX_FILE_SIZE_MB", 50),
		InstagramCookiesPath:        env("INSTAGRAM_COOKIES_PATH", "/secrets/instagram_cookies.txt"),
		InstagramCookiesFromBrowser: env("INSTAGRAM_COOKIES_FROM_BROWSER", ""),
		TikTokCarouselMaxItems:    envInt("TIKTOK_CAROUSEL_MAX_ITEMS", 20),
		TikTokCarouselAudioEnabled: envBool("TIKTOK_CAROUSEL_AUDIO_ENABLED", true),
		SpotifyEnabled:              envBool("SPOTIFY_ENABLED", false),
		SpotifyClientID:             os.Getenv("SPOTIFY_CLIENT_ID"),
		SpotifyClientSecret:         os.Getenv("SPOTIFY_CLIENT_SECRET"),
		SpotifyAPITimeoutSeconds:    envInt("SPOTIFY_API_TIMEOUT_SECONDS", 15),
		SpotifyDownloadEnabled:      envBool("SPOTIFY_DOWNLOAD_ENABLED", true),
		SpotifyTrackTimeoutSeconds:  envInt("SPOTIFY_TRACK_TIMEOUT_SECONDS", 60),
		SpotifyDLOutputFormat:       env("SPOTIFY_DL_OUTPUT_FORMAT", "mp3"),
		SpotifyLockMaxTracks:        envInt("SPOTIFY_LOCK_MAX_TRACKS", 50),
		SpotifyDownloadConcurrency:  envInt("SPOTIFY_DOWNLOAD_CONCURRENCY", 2),
		SoundCloudEnabled:             envBool("SOUNDCLOUD_ENABLED", true),
		SoundCloudDownloadEnabled:     envBool("SOUNDCLOUD_DOWNLOAD_ENABLED", false),
		SoundCloudTrackTimeoutSeconds: envInt("SOUNDCLOUD_TRACK_TIMEOUT_SECONDS", 30),
		SoundCloudMaxTracks:           envInt("SOUNDCLOUD_MAX_TRACKS", 100),
		SoundCloudDLOutputFormat:      env("SOUNDCLOUD_DL_OUTPUT_FORMAT", "mp3"),
		SoundCloudDownloadConcurrency: envInt("SOUNDCLOUD_DOWNLOAD_CONCURRENCY", 1),
		YandexMusicEnabled:             envBool("YANDEX_MUSIC_ENABLED", false),
		YandexMusicAccessToken:         os.Getenv("YANDEX_MUSIC_ACCESS_TOKEN"),
		YandexMusicAPITimeoutSeconds:   envInt("YANDEX_MUSIC_API_TIMEOUT_SECONDS", 15),
		YandexMusicDownloadEnabled:     envBool("YANDEX_MUSIC_DOWNLOAD_ENABLED", true),
		YandexMusicTrackTimeoutSeconds: envInt("YANDEX_MUSIC_TRACK_TIMEOUT_SECONDS", 60),
		YandexMusicDLOutputFormat:      env("YANDEX_MUSIC_DL_OUTPUT_FORMAT", "mp3"),
		YandexMusicLockMaxTracks:       envInt("YANDEX_MUSIC_LOCK_MAX_TRACKS", 50),
		YandexMusicDownloadConcurrency: envInt("YANDEX_MUSIC_DOWNLOAD_CONCURRENCY", 2),
		MetricsEnabled:         envBool("METRICS_ENABLED", true),
		MetricsHost:            env("METRICS_HOST", "0.0.0.0"),
		MetricsPort:            envInt("METRICS_PORT", 9101),
		WorkerMetricsPort:      envInt("WORKER_METRICS_PORT", 9102),
		AdminTelegramID:        envInt64("ADMIN_TELEGRAM_ID", 0),
		DownloadAPIEnabled:     envBool("DOWNLOAD_API_ENABLED", true),
		InternalAPIToken:         os.Getenv("INTERNAL_API_TOKEN"),
		InternalAPIRatePerMinute: envInt("INTERNAL_API_RATE_PER_MINUTE", 10),
		Mode:                   strings.ToLower(env("SAVEINATOR_MODE", "all")),
	}

	if s.BotToken == "" {
		return nil, fmt.Errorf("BOT_TOKEN is required")
	}

	if s.DownloadAPIEnabled && s.InternalAPIToken == "" {
		return nil, fmt.Errorf("INTERNAL_API_TOKEN is required when DOWNLOAD_API_ENABLED is true")
	}

	s.DatabaseURL = normalizePostgresURL(s.DatabaseURL)

	// Enable Spotify metadata when credentials are present unless explicitly disabled.
	if s.SpotifyClientID != "" && s.SpotifyClientSecret != "" && os.Getenv("SPOTIFY_ENABLED") == "" {
		s.SpotifyEnabled = true
	}

	// Enable Yandex Music when an access token is present unless explicitly disabled.
	if s.YandexMusicAccessToken != "" && os.Getenv("YANDEX_MUSIC_ENABLED") == "" {
		s.YandexMusicEnabled = true
	}

	return s, nil
}

func loadDotEnv() {
	for _, path := range []string{".env.go.dev", ".env", "../.env.go.dev", "../.env"} {
		if _, err := os.Stat(path); err == nil {
			_ = godotenv.Load(path)
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
