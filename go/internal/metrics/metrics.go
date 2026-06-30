package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	startTime       = time.Now()
	workerStartTime = time.Now()

	DownloadPlatforms = []string{
		"youtube", "tiktok", "instagram", "x", "pinterest", "spotify", "soundcloud",
	}
	YtdlpPlatforms = []string{
		"youtube", "tiktok", "instagram", "x", "pinterest",
	}

	UptimeSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "saveinator_uptime_seconds",
		Help: "Bot process uptime in seconds",
	})
	WorkerUptimeSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "saveinator_worker_uptime_seconds",
		Help: "Celery worker process uptime in seconds",
	})
	ActiveChats = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "saveinator_active_chats",
		Help: "Recently active unique chat IDs",
	})
	ActiveUsers = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "saveinator_active_users",
		Help: "Recently active unique Telegram user IDs",
	})

	MessagesReceivedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "saveinator_messages_received_total",
		Help: "Telegram messages received",
	})
	CommandsHandledTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "saveinator_commands_handled_total",
		Help: "Bot commands handled",
	}, []string{"command"})
	ErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "saveinator_errors_total",
		Help: "Unhandled errors and failures",
	}, []string{"source"})
	UsersCreatedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "saveinator_users_created_total",
		Help: "Users created in the Saveinator database",
	})

	TelegramAPIRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "saveinator_telegram_api_requests_total",
		Help: "Telegram Bot API requests",
	}, []string{"method", "status"})
	TelegramAPILatencySeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "saveinator_telegram_api_latency_seconds",
		Help:    "Telegram Bot API request latency",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
	}, []string{"method"})
	TelegramAPIFailuresTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "saveinator_telegram_api_failures_total",
		Help: "Failed Telegram Bot API requests",
	})

	HTTPRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "saveinator_http_requests_total",
		Help: "HTTP API requests by method, route, and status",
	}, []string{"method", "route", "status"})
	HTTPRequestLatencySeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "saveinator_http_request_latency_seconds",
		Help:    "HTTP API request latency by method and route",
		Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0},
	}, []string{"method", "route"})

	DownloadsEnqueued = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "saveinator_downloads_enqueued_total",
		Help: "Download jobs started by platform",
	}, []string{"platform"})
	RateLimitDropped = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "saveinator_rate_limit_dropped_total",
		Help: "Messages dropped by rate limiter",
	}, []string{"scope"})
	UserQueueRejectedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "saveinator_user_queue_rejected_total",
		Help: "Download requests rejected because the user already has an active scenario",
	}, []string{"scenario"})
	SpamBlocked = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "saveinator_spam_blocked_total",
		Help: "Messages blocked by spam middleware",
	}, []string{"reason"})

	SpotifyRequestsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "saveinator_spotify_requests_total",
		Help: "Spotify link handling requests",
	})
	SoundCloudRequestsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "saveinator_soundcloud_requests_total",
		Help: "SoundCloud link handling requests",
	})
	SoundCloudDownloadsEnqueuedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "saveinator_soundcloud_downloads_enqueued_total",
		Help: "SoundCloud track downloads started",
	})
	SoundCloudDownloadsSuccessTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "saveinator_soundcloud_downloads_success_total",
		Help: "Successful SoundCloud track downloads",
	})
	SoundCloudDownloadFailuresTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "saveinator_soundcloud_download_failures_total",
		Help: "Failed SoundCloud track downloads",
	})
	SoundCloudDownloadsTimeoutTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "saveinator_soundcloud_download_timeouts_total",
		Help: "SoundCloud track download timeouts",
	})
	SoundCloudMetadataFailuresTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "saveinator_soundcloud_metadata_failures_total",
		Help: "SoundCloud metadata fetch failures",
	})
	SoundCloudPlaylistTracks = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "saveinator_soundcloud_playlist_tracks",
		Help:    "Number of tracks in SoundCloud playlists",
		Buckets: []float64{1, 2, 5, 10, 15, 20, 30, 50},
	})
	SoundCloudDownloadDurationSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "saveinator_soundcloud_download_duration_seconds",
		Help:    "SoundCloud track download duration",
		Buckets: []float64{1, 3, 5, 10, 15, 30, 60, 120},
	})
	SoundCloudYouTubeFallbackTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "saveinator_soundcloud_youtube_fallback_total",
		Help: "SoundCloud tracks downloaded via YouTube fallback",
	})

	TikTokCarouselRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "saveinator_tiktok_carousel_requests_total",
		Help: "TikTok carousel download requests",
	}, []string{"status"})
	TikTokCarouselImagesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "saveinator_tiktok_carousel_images_total",
		Help: "Images sent from TikTok carousels",
	})
	TikTokCarouselAudioTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "saveinator_tiktok_carousel_audio_total",
		Help: "Audio tracks sent from TikTok carousels",
	})
	TikTokCarouselFailuresTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "saveinator_tiktok_carousel_failures_total",
		Help: "TikTok carousel failures",
	}, []string{"reason"})

	BroadcastsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "saveinator_broadcasts_total",
		Help: "Broadcasts created",
	}, []string{"audience"})
	BroadcastMessagesSentTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "saveinator_broadcast_messages_sent_total",
		Help: "Broadcast messages successfully sent",
	})
	BroadcastMessagesFailedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "saveinator_broadcast_messages_failed_total",
		Help: "Broadcast messages that failed to send",
	})
	AdminRuntimeSettingsChangedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "saveinator_admin_runtime_settings_changed_total",
		Help: "Admin runtime setting changes",
	}, []string{"service"})
	RPCFailuresTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "saveinator_rpc_failures_total",
		Help: "Infrastructure RPC failures (database, redis, internal)",
	}, []string{"rpc_type"})

	CeleryTasksTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "saveinator_celery_tasks_total",
		Help: "Celery tasks by name and status",
	}, []string{"task", "status"})
	DownloadDurationSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "saveinator_download_duration_seconds",
		Help:    "Video download task duration",
		Buckets: []float64{1, 5, 15, 30, 60, 120, 300, 600},
	}, []string{"platform"})
	DownloadFileSizeBytes = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "saveinator_download_file_size_bytes",
		Help: "Downloaded file sizes by platform",
		Buckets: []float64{
			1 * 1024 * 1024,
			5 * 1024 * 1024,
			10 * 1024 * 1024,
			25 * 1024 * 1024,
			50 * 1024 * 1024,
			100 * 1024 * 1024,
			250 * 1024 * 1024,
			500 * 1024 * 1024,
			1024 * 1024 * 1024,
			2 * 1024 * 1024 * 1024,
		},
	}, []string{"platform"})
	YtdlpErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "saveinator_ytdlp_errors_total",
		Help: "yt-dlp download failures",
	}, []string{"platform"})

	WorkerBroadcastsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "saveinator_worker_broadcasts_total",
		Help: "Broadcast tasks executed by worker",
	}, []string{"audience"})
	WorkerBroadcastMessagesSentTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "saveinator_worker_broadcast_messages_sent_total",
		Help: "Broadcast messages sent by worker",
	})
	WorkerBroadcastMessagesFailedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "saveinator_worker_broadcast_messages_failed_total",
		Help: "Broadcast messages that failed in worker",
	})
	WorkerBroadcastMessagesBlockedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "saveinator_worker_broadcast_messages_blocked_total",
		Help: "Users who blocked the bot during broadcast",
	})
)

func init() {
	prometheus.MustRegister(
		UptimeSeconds, WorkerUptimeSeconds, ActiveChats, ActiveUsers,
		MessagesReceivedTotal, CommandsHandledTotal, ErrorsTotal, UsersCreatedTotal,
		TelegramAPIRequestsTotal, TelegramAPILatencySeconds, TelegramAPIFailuresTotal,
		HTTPRequestsTotal, HTTPRequestLatencySeconds,
		DownloadsEnqueued, RateLimitDropped, UserQueueRejectedTotal, SpamBlocked,
		SpotifyRequestsTotal,
		SoundCloudRequestsTotal, SoundCloudDownloadsEnqueuedTotal, SoundCloudDownloadsSuccessTotal,
		SoundCloudDownloadFailuresTotal, SoundCloudDownloadsTimeoutTotal,
		SoundCloudMetadataFailuresTotal, SoundCloudPlaylistTracks, SoundCloudDownloadDurationSeconds,
		SoundCloudYouTubeFallbackTotal,
		TikTokCarouselRequestsTotal, TikTokCarouselImagesTotal, TikTokCarouselAudioTotal, TikTokCarouselFailuresTotal,
		BroadcastsTotal, BroadcastMessagesSentTotal, BroadcastMessagesFailedTotal,
		AdminRuntimeSettingsChangedTotal, RPCFailuresTotal,
		CeleryTasksTotal, DownloadDurationSeconds, DownloadFileSizeBytes, YtdlpErrorsTotal,
		WorkerBroadcastsTotal, WorkerBroadcastMessagesSentTotal,
		WorkerBroadcastMessagesFailedTotal, WorkerBroadcastMessagesBlockedTotal,
	)
	initPlatformMetrics()
}

func initPlatformMetrics() {
	for _, platform := range DownloadPlatforms {
		DownloadsEnqueued.WithLabelValues(platform)
	}
	for _, platform := range YtdlpPlatforms {
		YtdlpErrorsTotal.WithLabelValues(platform)
		DownloadDurationSeconds.WithLabelValues(platform)
		DownloadFileSizeBytes.WithLabelValues(platform)
	}
}

func Handler() http.Handler {
	base := promhttp.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Refresh()
		base.ServeHTTP(w, r)
	})
}

func Refresh() {
	UptimeSeconds.Set(time.Since(startTime).Seconds())
	WorkerUptimeSeconds.Set(time.Since(workerStartTime).Seconds())
	refreshActiveChats()
}
