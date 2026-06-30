package metrics

import (
	"errors"
	"testing"
	"time"
)

func TestObserveTelegramAPISuccess(t *testing.T) {
	started := time.Now().Add(-50 * time.Millisecond)
	ObserveTelegramAPI("sendMessage", nil, started)
}

func TestObserveTelegramAPIError(t *testing.T) {
	started := time.Now().Add(-50 * time.Millisecond)
	ObserveTelegramAPI("sendMessage", errors.New("fail"), started)
}

func TestCallTelegram(t *testing.T) {
	called := false
	err := CallTelegram("getMe", func() error {
		called = true
		return nil
	})
	if err != nil || !called {
		t.Fatalf("CallTelegram err=%v called=%v", err, called)
	}
}
