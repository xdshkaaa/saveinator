package handler

import (
	"github.com/mymmrac/telego"

	"saveinator/internal/queue"
)

type taskEnqueuer interface {
	EnqueueDownload(p queue.DownloadPayload) error
	EnqueueTikTok(p queue.DownloadPayload) error
	EnqueuePinterestDefault(p queue.DownloadPayload) error
	EnqueueSpotify(p queue.MusicPayload) error
	EnqueueSoundCloud(p queue.MusicPayload) error
	EnqueueBroadcast(p queue.BroadcastPayload) error
	EnqueueTikTokCarousel(p queue.DownloadPayload) error
	EnqueueInstagram(p queue.DownloadPayload) error
}

type messageSender interface {
	SendMessage(params *telego.SendMessageParams) (*telego.Message, error)
}

func sceneForPlatform(platform string) string {
	switch platform {
	case "tiktok":
		return "tiktok"
	case "spotify":
		return "spotify"
	case "soundcloud":
		return "soundcloud"
	default:
		return platform
	}
}

func shouldAcquireUserLock(userID int64, batch bool, adminID int64) bool {
	if batch {
		return false
	}
	if adminID != 0 && userID == adminID {
		return false
	}
	return true
}
