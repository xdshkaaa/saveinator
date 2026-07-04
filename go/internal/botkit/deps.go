package botkit

import (
	"github.com/mymmrac/telego"
)

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
