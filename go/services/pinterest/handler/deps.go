package handler

import (
	"github.com/mymmrac/telego"

	"saveinator/internal/queue"
)

type taskEnqueuer interface {
	EnqueuePinterest(p queue.DownloadPayload) error
	EnqueueBroadcast(p queue.BroadcastPayload) error
}

type messageSender interface {
	SendMessage(params *telego.SendMessageParams) (*telego.Message, error)
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
