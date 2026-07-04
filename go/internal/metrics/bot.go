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
)

func init() {
	prometheus.MustRegister(BotUpdatesTotal, BotDownloadsEnqueuedTotal, BotTasksTotal)
}
