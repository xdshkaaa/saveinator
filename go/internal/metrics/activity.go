package metrics

import (
	"sync"
	"time"

	"github.com/mymmrac/telego"
)

const activeWindow = 30 * time.Minute

var (
	activeMu        sync.Mutex
	activeChatIDs   = map[int64]time.Time{}
	activeUserIDs   = map[int64]time.Time{}
)

func RecordMessage(chatID int64, userID int64) {
	MessagesReceivedTotal.Inc()
	now := time.Now()
	activeMu.Lock()
	defer activeMu.Unlock()
	if chatID != 0 {
		activeChatIDs[chatID] = now
	}
	if userID != 0 {
		activeUserIDs[userID] = now
	}
}

func RecordUpdate(update telego.Update) {
	var chatID, userID int64
	switch {
	case update.Message != nil:
		chatID = update.Message.Chat.ID
		if update.Message.From != nil {
			userID = update.Message.From.ID
		}
	case update.EditedMessage != nil:
		chatID = update.EditedMessage.Chat.ID
		if update.EditedMessage.From != nil {
			userID = update.EditedMessage.From.ID
		}
	case update.CallbackQuery != nil:
		if update.CallbackQuery.Message != nil {
			chatID = update.CallbackQuery.Message.GetChat().ID
		}
		userID = update.CallbackQuery.From.ID
	}
	if chatID != 0 || userID != 0 {
		RecordMessage(chatID, userID)
	}
}

func ActiveUserCount() int {
	refreshActiveChats()
	activeMu.Lock()
	defer activeMu.Unlock()
	return len(activeUserIDs)
}

func refreshActiveChats() {
	cutoff := time.Now().Add(-activeWindow)
	activeMu.Lock()
	defer activeMu.Unlock()
	for id, last := range activeChatIDs {
		if last.Before(cutoff) {
			delete(activeChatIDs, id)
		}
	}
	for id, last := range activeUserIDs {
		if last.Before(cutoff) {
			delete(activeUserIDs, id)
		}
	}
	ActiveChats.Set(float64(len(activeChatIDs)))
	ActiveUsers.Set(float64(len(activeUserIDs)))
}
