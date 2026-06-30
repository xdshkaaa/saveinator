package metrics

import (
	"testing"

	"github.com/mymmrac/telego"
)

func TestRecordUpdateMessage(t *testing.T) {
	RecordUpdate(telego.Update{
		Message: &telego.Message{
			Chat: telego.Chat{ID: 100},
			From: &telego.User{ID: 200},
		},
	})
	if ActiveUserCount() < 1 {
		t.Fatal("expected at least one active user")
	}
}

func TestRecordUpdateCallback(t *testing.T) {
	RecordUpdate(telego.Update{
		CallbackQuery: &telego.CallbackQuery{
			From: telego.User{ID: 300},
			Message: &telego.Message{
				Chat: telego.Chat{ID: 400},
			},
		},
	})
	if ActiveUserCount() < 1 {
		t.Fatal("expected active users after callback")
	}
}
