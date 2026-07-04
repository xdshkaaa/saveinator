package metrics

import "github.com/prometheus/client_golang/prometheus"

// Per-bot metrics for multi-bot (botd) deployments; the bot label carries the
// BotConfig slug.
var (
	BotUpdatesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "saveinator_bot_updates_total",
		Help: "Telegram updates received per bot",
	}, []string{"bot"})
	BotDownloadsEnqueuedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "saveinator_bot_downloads_enqueued_total",
		Help: "Download jobs started per bot and platform",
	}, []string{"bot", "platform"})
	BotTasksTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "saveinator_bot_tasks_total",
		Help: "Worker task results per bot and status",
	}, []string{"bot", "status"})
	BotTaskDurationSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "saveinator_bot_task_duration_seconds",
		Help:    "Worker task duration per bot",
		Buckets: []float64{1, 5, 15, 30, 60, 120, 300, 600},
	}, []string{"bot"})
)

func init() {
	prometheus.MustRegister(BotUpdatesTotal, BotDownloadsEnqueuedTotal, BotTasksTotal, BotTaskDurationSeconds)
}

// InitBotMetrics pre-creates per-bot series so dashboards show zeros instead
// of gaps for bots that have not seen traffic yet.
func InitBotMetrics(slugs []string) {
	for _, slug := range slugs {
		BotUpdatesTotal.WithLabelValues(slug)
		BotTasksTotal.WithLabelValues(slug, "SUCCESS")
		BotTasksTotal.WithLabelValues(slug, "FAILURE")
		BotTaskDurationSeconds.WithLabelValues(slug)
	}
}
