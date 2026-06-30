package metrics

import (
	"testing"

	"saveinator/internal/queue"
)

func TestCeleryTaskName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		taskType string
		want     string
	}{
		{taskType: queue.TypeDownload, want: "workers.tasks.download_and_send_task"},
		{taskType: queue.TypePinterest, want: "workers.pinterest_task.pinterest_download_task"},
		{taskType: queue.TypeTikTok, want: "workers.tiktok_task.tiktok_download_task"},
		{taskType: queue.TypeTikTokCarousel, want: "workers.tiktok_task.tiktok_carousel_images_task"},
		{taskType: queue.TypeSpotify, want: "workers.tasks.spotify_download_task"},
		{taskType: queue.TypeSoundCloud, want: "workers.tasks.soundcloud_download_task"},
		{taskType: queue.TypeBroadcast, want: "workers.broadcast_task.execute_broadcast"},
		{taskType: "custom:task", want: "custom:task"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.taskType, func(t *testing.T) {
			t.Parallel()
			if got := CeleryTaskName(tt.taskType); got != tt.want {
				t.Fatalf("CeleryTaskName(%q) = %q, want %q", tt.taskType, got, tt.want)
			}
		})
	}
}
