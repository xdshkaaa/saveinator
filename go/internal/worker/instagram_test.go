package worker

import (
	"context"
	"testing"

	"github.com/hibiken/asynq"

	"saveinator/internal/queue"
)

func TestHandleInstagram_invalidPayload(t *testing.T) {
	t.Parallel()
	h, _, _ := testWorkerHandler(t)
	task := asynq.NewTask(queue.TypeInstagram, []byte("not-json"))
	if err := h.handleInstagram(context.Background(), task); err == nil {
		t.Fatal("expected unmarshal error")
	}
}
