package handler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	"github.com/redis/go-redis/v9"

	"saveinator/internal/config"
	"saveinator/internal/linkparser"
	"saveinator/internal/queue"
	"saveinator/internal/redisx"
	"saveinator/internal/runtime"
)

type recordingQueue struct {
	mu    sync.Mutex
	calls []string
	last  queue.DownloadPayload
	err   error
}

func (q *recordingQueue) EnqueueDownload(p queue.DownloadPayload) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.calls = append(q.calls, queue.TypeDownload)
	q.last = p
	return q.err
}

func (q *recordingQueue) EnqueueTikTok(p queue.DownloadPayload) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.calls = append(q.calls, queue.TypeTikTok)
	q.last = p
	return q.err
}

func (q *recordingQueue) EnqueueSpotify(p queue.MusicPayload) error           { return nil }
func (q *recordingQueue) EnqueueSoundCloud(p queue.MusicPayload) error        { return nil }
func (q *recordingQueue) EnqueueBroadcast(p queue.BroadcastPayload) error     { return nil }
func (q *recordingQueue) EnqueueTikTokCarousel(p queue.DownloadPayload) error { return nil }
func (q *recordingQueue) EnqueuePinterestDefault(p queue.DownloadPayload) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.calls = append(q.calls, queue.TypePinterest)
	q.last = p
	return q.err
}

func (q *recordingQueue) EnqueueInstagram(p queue.DownloadPayload) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.calls = append(q.calls, queue.TypeInstagram)
	q.last = p
	return q.err
}

type stubMessenger struct {
	messageID int
}

func (s *stubMessenger) SendMessage(params *telego.SendMessageParams) (*telego.Message, error) {
	s.messageID++
	return &telego.Message{MessageID: s.messageID}, nil
}

func testHandlerBot(t *testing.T, adminID int64) (*Bot, *recordingQueue, *redisx.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	redisClient := redisx.NewWithRedis(rdb)
	recQ := &recordingQueue{}
	cfg := &config.Settings{
		AdminTelegramID:        adminID,
		DownloadTimeoutSeconds: 60,
		RateLimitUserPerMinute: 100,
		RateLimitChatPerMinute: 100,
	}
	bot := &Bot{
		cfg:     cfg,
		redis:   redisClient,
		q:       recQ,
		runtime: runtime.NewStore(redisClient, cfg),
	}
	return bot, recQ, redisClient
}

func TestEnqueue_tiktokWithLock(t *testing.T) {
	b, recQ, _ := testHandlerBot(t, 0)
	ctx := context.Background()
	messenger := &stubMessenger{}
	userID := int64(42)
	msg := telego.Message{
		Chat: telego.Chat{ID: 1, Type: "private"},
		From: &telego.User{ID: userID},
	}
	link := linkparser.ParsedLink{
		Platform: linkparser.PlatformTikTok,
		URL:      "https://www.tiktok.com/@u/video/123",
	}

	if err := b.enqueue(ctx, messenger, msg, "en", link, "tiktok", queue.TypeTikTok, false); err != nil {
		t.Fatal(err)
	}
	recQ.mu.Lock()
	defer recQ.mu.Unlock()
	if len(recQ.calls) != 1 || recQ.calls[0] != queue.TypeTikTok {
		t.Fatalf("calls = %v, want tiktok enqueue", recQ.calls)
	}
	if recQ.last.LockToken == "" || recQ.last.LockScene != "tiktok" {
		t.Fatalf("payload lock = %+v", recQ.last)
	}
}

func TestEnqueue_instagramWithLock(t *testing.T) {
	b, recQ, _ := testHandlerBot(t, 0)
	ctx := context.Background()
	messenger := &stubMessenger{}
	userID := int64(42)
	msg := telego.Message{
		Chat: telego.Chat{ID: 1, Type: "private"},
		From: &telego.User{ID: userID},
	}
	link := linkparser.ParsedLink{
		Platform: linkparser.PlatformInstagram,
		URL:      "https://www.instagram.com/reel/CxAbC12345/",
	}

	if err := b.enqueue(ctx, messenger, msg, "en", link, "instagram", queue.TypeInstagram, false); err != nil {
		t.Fatal(err)
	}
	recQ.mu.Lock()
	defer recQ.mu.Unlock()
	if len(recQ.calls) != 1 || recQ.calls[0] != queue.TypeInstagram {
		t.Fatalf("calls = %v, want instagram enqueue", recQ.calls)
	}
	if recQ.last.LockToken == "" || recQ.last.LockScene != "instagram" {
		t.Fatalf("payload lock = %+v", recQ.last)
	}
	if recQ.last.Platform != "instagram" {
		t.Fatalf("platform = %q, want instagram", recQ.last.Platform)
	}
}

func TestEnqueue_batchSkipsLock(t *testing.T) {
	b, recQ, redisClient := testHandlerBot(t, 0)
	ctx := context.Background()
	messenger := &stubMessenger{}
	msg := telego.Message{
		Chat: telego.Chat{ID: 1, Type: "private"},
		From: &telego.User{ID: 42},
	}
	link := linkparser.ParsedLink{
		Platform: linkparser.PlatformTikTok,
		URL:      "https://www.tiktok.com/@u/video/123",
	}

	if err := b.enqueue(ctx, messenger, msg, "en", link, "tiktok", queue.TypeTikTok, true); err != nil {
		t.Fatal(err)
	}
	if recQ.last.LockToken != "" {
		t.Fatalf("batch should not set lock token, got %q", recQ.last.LockToken)
	}
	active, err := redisClient.GetActiveDownload(ctx, 42)
	if err != nil || active != nil {
		t.Fatal("batch should not acquire user lock")
	}
}

func TestEnqueue_userBusy(t *testing.T) {
	b, recQ, redisClient := testHandlerBot(t, 0)
	ctx := context.Background()
	messenger := &stubMessenger{}
	userID := int64(7)
	if _, ok, err := redisClient.AcquireUserLock(ctx, userID, "tiktok", time.Minute); err != nil || !ok {
		t.Fatal("setup lock failed")
	}
	msg := telego.Message{
		Chat: telego.Chat{ID: 1, Type: "private"},
		From: &telego.User{ID: userID},
	}
	link := linkparser.ParsedLink{
		Platform: linkparser.PlatformTikTok,
		URL:      "https://www.tiktok.com/@u/video/999",
	}

	if err := b.enqueue(ctx, messenger, msg, "en", link, "tiktok", queue.TypeTikTok, false); err != nil {
		t.Fatal(err)
	}
	if len(recQ.calls) != 0 {
		t.Fatal("enqueue should not run when user is busy")
	}
}

func TestEnqueue_adminSkipsLock(t *testing.T) {
	const adminID int64 = 99
	b, recQ, redisClient := testHandlerBot(t, adminID)
	ctx := context.Background()
	messenger := &stubMessenger{}
	msg := telego.Message{
		Chat: telego.Chat{ID: 1, Type: "private"},
		From: &telego.User{ID: adminID},
	}
	link := linkparser.ParsedLink{
		Platform: linkparser.PlatformTikTok,
		URL:      "https://www.tiktok.com/@u/video/1",
	}

	if err := b.enqueue(ctx, messenger, msg, "en", link, "tiktok", queue.TypeTikTok, false); err != nil {
		t.Fatal(err)
	}
	if recQ.last.LockToken != "" {
		t.Fatal("admin should skip lock token")
	}
	active, _ := redisClient.GetActiveDownload(ctx, adminID)
	if active != nil {
		t.Fatal("admin should not hold user lock")
	}
}

func TestAllowRateLimit_exceeded(t *testing.T) {
	b, _, _ := testHandlerBot(t, 0)
	ctx := context.Background()
	b.cfg.RateLimitUserPerMinute = 1
	b.cfg.RateLimitChatPerMinute = 100

	msg := telego.Message{
		Chat: telego.Chat{ID: 1, Type: "private"},
		From: &telego.User{ID: 55},
	}
	messenger := &stubMessenger{}
	if !b.allowRateLimit(ctx, messenger, msg, "en") {
		t.Fatal("first request should pass")
	}
	if b.allowRateLimit(ctx, messenger, msg, "en") {
		t.Fatal("second request should be rate limited")
	}
}

// stubMessenger implements the subset of telego.Bot used by enqueue.
var _ interface {
	SendMessage(*telego.SendMessageParams) (*telego.Message, error)
} = (*stubMessenger)(nil)

func TestEnqueue_usesStubMessenger(t *testing.T) {
	t.Parallel()
	messenger := &stubMessenger{}
	_, err := messenger.SendMessage(tu.Message(tu.ID(1), "ok"))
	if err != nil {
		t.Fatal(err)
	}
}
