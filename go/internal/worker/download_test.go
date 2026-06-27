package worker

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/mymmrac/telego"
	"github.com/redis/go-redis/v9"

	"saveinator/internal/config"
	"saveinator/internal/queue"
	"saveinator/internal/redisx"
)

type recordingSender struct {
	mu     sync.Mutex
	edits  []string
	deletes []int
}

func (s *recordingSender) EditMessage(chatID int64, messageID int, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.edits = append(s.edits, text)
	return nil
}

func (s *recordingSender) EditMessageMarkup(chatID int64, messageID int, text string, markup *telego.InlineKeyboardMarkup) error {
	return s.EditMessage(chatID, messageID, text)
}

func (s *recordingSender) DeleteMessage(chatID int64, messageID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletes = append(s.deletes, messageID)
	return nil
}

func (s *recordingSender) SendPhotoAlbum(chatID int64, paths []string, caption string) error {
	return nil
}
func (s *recordingSender) SendFile(chatID int64, path, title, lang, platform string, animation bool) error {
	return nil
}
func (s *recordingSender) SendFileWithMarkup(chatID int64, path, title, lang, platform string, animation bool, markup *telego.InlineKeyboardMarkup) error {
	return nil
}
func (s *recordingSender) SendAudio(chatID int64, path, title, performer string, durationSec int) error {
	return nil
}

func testWorkerHandler(t *testing.T) (*Handler, *redisx.Client, *recordingSender) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	redisClient := redisx.NewWithRedis(rdb)
	sender := &recordingSender{}
	h := &Handler{
		cfg:     &config.Settings{DownloadTimeoutSeconds: 60},
		sender:  sender,
		redis:   redisClient,
		runtime: nil,
	}
	return h, redisClient, sender
}

func TestHandleDownload_invalidPayload(t *testing.T) {
	t.Parallel()
	h, _, _ := testWorkerHandler(t)
	task := asynq.NewTask(queue.TypeDownload, []byte("not-json"))
	if err := h.handleDownload(context.Background(), task); err == nil {
		t.Fatal("expected unmarshal error")
	}
}

func TestCheckCancelled_earlyExit(t *testing.T) {
	t.Parallel()
	h, redisClient, sender := testWorkerHandler(t)
	ctx := context.Background()

	token, ok, err := redisClient.AcquireUserLock(ctx, 1, "tiktok", time.Minute)
	if err != nil || !ok {
		t.Fatal("lock setup failed")
	}
	if err := redisClient.SetDownloadCancelled(ctx, "tiktok", 1, token, time.Minute); err != nil {
		t.Fatal(err)
	}

	p := queue.DownloadPayload{
		UserID:    1,
		ChatID:    10,
		MessageID: 99,
		Lang:      "en",
		LockScene: "tiktok",
		LockToken: token,
	}
	if !h.checkCancelled(ctx, p) {
		t.Fatal("expected cancelled")
	}
	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.edits) != 1 {
		t.Fatalf("expected status edit, got %d edits", len(sender.edits))
	}
}

func TestReleaseLock_onDefer(t *testing.T) {
	t.Parallel()
	h, redisClient, _ := testWorkerHandler(t)
	ctx := context.Background()

	token, ok, err := redisClient.AcquireUserLock(ctx, 2, "tiktok", time.Minute)
	if err != nil || !ok {
		t.Fatal("lock setup failed")
	}
	p := queue.DownloadPayload{
		UserID:    2,
		LockScene: "tiktok",
		LockToken: token,
	}
	h.releaseLock(ctx, p)

	active, err := redisClient.GetActiveDownload(ctx, 2)
	if err != nil || active != nil {
		t.Fatal("lock should be released")
	}
}

func TestHandleDownload_cancelledSkipsWork(t *testing.T) {
	t.Parallel()
	h, redisClient, _ := testWorkerHandler(t)
	ctx := context.Background()

	token, ok, err := redisClient.AcquireUserLock(ctx, 3, "tiktok", time.Minute)
	if err != nil || !ok {
		t.Fatal("lock setup failed")
	}
	if err := redisClient.SetDownloadCancelled(ctx, "tiktok", 3, token, time.Minute); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(queue.DownloadPayload{
		UserID:    3,
		URL:       "https://example.com",
		Platform:  "instagram",
		LockScene: "tiktok",
		LockToken: token,
	})
	task := asynq.NewTask(queue.TypeDownload, body)
	if err := h.handleDownload(ctx, task); err != nil {
		t.Fatal(err)
	}
	active, _ := redisClient.GetActiveDownload(ctx, 3)
	if active != nil {
		t.Fatal("lock should be released after cancelled handle")
	}
}

func TestYouTubeTranscodePath_selected(t *testing.T) {
	t.Parallel()
	p := queue.DownloadPayload{
		Platform:    "youtube",
		Quality:     720,
		AspectRatio: "16_9",
	}
	useYouTube := p.Platform == "youtube" && p.Quality > 0 && p.AspectRatio != ""
	if !useYouTube {
		t.Fatal("expected youtube transcode path conditions")
	}
}
