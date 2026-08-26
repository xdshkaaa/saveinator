package metrics

import (
	"time"

	"saveinator/internal/queue"
)

func RecordCommand(command string) {
	CommandsHandledTotal.WithLabelValues(command).Inc()
}

func RecordError(source string) {
	ErrorsTotal.WithLabelValues(source).Inc()
}

func RecordUserCreated() {
	UsersCreatedTotal.Inc()
}

func RecordRPCFailure(rpcType string) {
	RPCFailuresTotal.WithLabelValues(rpcType).Inc()
}

func RecordUserQueueRejected(scenario string) {
	UserQueueRejectedTotal.WithLabelValues(scenario).Inc()
}

func RecordAdminRuntimeSetting(service string) {
	AdminRuntimeSettingsChangedTotal.WithLabelValues(service).Inc()
}

func RecordBroadcastCreated(audience string) {
	BroadcastsTotal.WithLabelValues(audience).Inc()
}

func RecordYtdlpError(platform string) {
	YtdlpErrorsTotal.WithLabelValues(platform).Inc()
}

func ObserveDownloadDuration(platform string, d time.Duration) {
	DownloadDurationSeconds.WithLabelValues(platform).Observe(d.Seconds())
}

func ObserveDownloadFileSize(platform string, bytes int64) {
	if bytes > 0 {
		DownloadFileSizeBytes.WithLabelValues(platform).Observe(float64(bytes))
	}
}

func CeleryTaskName(taskType string) string {
	switch taskType {
	case queue.TypeDownload:
		return "workers.tasks.download_and_send_task"
	case queue.TypePinterest:
		return "workers.pinterest_task.pinterest_download_task"
	case queue.TypeTikTok:
		return "workers.tiktok_task.tiktok_download_task"
	case queue.TypeTikTokCarousel:
		return "workers.tiktok_task.tiktok_carousel_images_task"
	case queue.TypeSpotify:
		return "workers.tasks.spotify_download_task"
	case queue.TypeYandexMusic:
		return "workers.tasks.yandexmusic_download_task"
	case queue.TypeSoundCloud:
		return "workers.tasks.soundcloud_download_task"
	case queue.TypeBroadcast:
		return "workers.broadcast_task.execute_broadcast"
	default:
		return taskType
	}
}

func RecordCeleryTask(taskType, status string) {
	CeleryTasksTotal.WithLabelValues(CeleryTaskName(taskType), status).Inc()
}

func RecordWorkerBroadcast(audience string, sent, failed, blocked int) {
	WorkerBroadcastsTotal.WithLabelValues(audience).Inc()
	if sent > 0 {
		WorkerBroadcastMessagesSentTotal.Add(float64(sent))
	}
	if failed > 0 {
		WorkerBroadcastMessagesFailedTotal.Add(float64(failed))
	}
	if blocked > 0 {
		WorkerBroadcastMessagesBlockedTotal.Add(float64(blocked))
	}
}
