package worker

import (
	"time"

	"saveinator/internal/metrics"
)

func recordTaskSuccess(taskType, platform string, start time.Time, fileBytes int64) {
	metrics.RecordCeleryTask(taskType, "SUCCESS")
	if platform != "" {
		metrics.ObserveDownloadDuration(platform, time.Since(start))
		if fileBytes > 0 {
			metrics.ObserveDownloadFileSize(platform, fileBytes)
		}
	}
}

func recordTaskFailure(taskType string) {
	metrics.RecordCeleryTask(taskType, "FAILURE")
}
