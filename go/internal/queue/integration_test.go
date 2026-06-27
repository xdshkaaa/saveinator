package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

func testRedisURL(t *testing.T) (string, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	return fmt.Sprintf("redis://%s/0", mr.Addr()), mr
}

func TestClearUserTasks_deletesPending(t *testing.T) {
	redisURL, _ := testRedisURL(t)
	client, err := NewClient(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	payload := DownloadPayload{
		URL:       "https://tiktok.com/@u/video/1",
		Platform:  "tiktok",
		ChatID:    1,
		UserID:    99,
		MessageID: 10,
		Lang:      "en",
		LockScene: "tiktok",
		LockToken: "abc",
	}
	if err := client.EnqueueTikTok(payload); err != nil {
		t.Fatal(err)
	}

	insp, err := NewInspector(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	defer insp.Close()

	result, _, err := ClearUserTasks(insp, 99)
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedPending != 1 {
		t.Fatalf("DeletedPending = %d, want 1", result.DeletedPending)
	}

	pending, err := insp.ListPendingTasks(defaultQueueName)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending tasks, got %d", len(pending))
	}
}

func TestClearUserTasks_ignoresOtherUsers(t *testing.T) {
	redisURL, _ := testRedisURL(t)
	client, err := NewClient(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	for _, uid := range []int64{1, 2} {
		p := DownloadPayload{URL: "https://x.com/i/status/1", UserID: uid, ChatID: 1}
		if err := client.EnqueueDownload(p); err != nil {
			t.Fatal(err)
		}
	}

	insp, err := NewInspector(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	defer insp.Close()

	result, _, err := ClearUserTasks(insp, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedPending != 1 {
		t.Fatalf("DeletedPending = %d, want 1", result.DeletedPending)
	}
	pending, err := insp.ListPendingTasks(defaultQueueName)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 remaining task, got %d", len(pending))
	}
}

func TestRecoverOrphanedActiveTasks_removesStaleEntry(t *testing.T) {
	redisURL, _ := testRedisURL(t)
	opt, err := RedisOpt(redisURL)
	if err != nil {
		t.Fatal(err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     opt.Addr,
		Password: opt.Password,
		DB:       opt.DB,
	})
	defer rdb.Close()

	ctx := context.Background()
	activeKey := fmt.Sprintf("asynq:{%s}:active", defaultQueue)
	leaseKey := fmt.Sprintf("asynq:{%s}:lease", defaultQueue)
	orphanID := "orphan-task-id"

	if err := rdb.RPush(ctx, activeKey, orphanID).Err(); err != nil {
		t.Fatal(err)
	}
	if err := rdb.ZAdd(ctx, leaseKey, redis.Z{Score: 9999999999, Member: orphanID}).Err(); err != nil {
		t.Fatal(err)
	}

	if err := RecoverOrphanedActiveTasks(redisURL); err != nil {
		t.Fatal(err)
	}

	ids, err := rdb.LRange(ctx, activeKey, 0, -1).Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("active list = %v, want empty", ids)
	}
}

func TestClearUserTasks_collectsLockFromActive(t *testing.T) {
	redisURL, _ := testRedisURL(t)
	opt, err := RedisOpt(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	insp, err := NewInspector(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	defer insp.Close()

	payload := DownloadPayload{
		URL:       "https://tiktok.com/@u/video/3",
		UserID:    77,
		LockScene: "tiktok",
		LockToken: "active-token",
	}
	body, _ := json.Marshal(payload)
	task := asynq.NewTask(TypeTikTok, body)
	if _, err := asynq.NewClient(opt).Enqueue(task); err != nil {
		t.Fatal(err)
	}

	pending, err := insp.ListPendingTasks(defaultQueueName)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending tasks = %d, err = %v", len(pending), err)
	}
	taskID := pending[0].ID

	rdb := redis.NewClient(&redis.Options{Addr: opt.Addr, Password: opt.Password, DB: opt.DB})
	defer rdb.Close()
	ctx := context.Background()
	activeKey := fmt.Sprintf("asynq:{%s}:active", defaultQueue)
	if err := rdb.RPush(ctx, activeKey, taskID).Err(); err != nil {
		t.Fatal(err)
	}

	_, lockRefs, err := ClearUserTasks(insp, 77)
	if err != nil {
		t.Fatal(err)
	}
	if len(lockRefs) != 1 || lockRefs[0].Token != "active-token" {
		t.Fatalf("lockRefs = %+v", lockRefs)
	}
}

func TestRecoverOrphanedActiveTasks_keepsValidPending(t *testing.T) {
	redisURL, _ := testRedisURL(t)
	client, err := NewClient(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	payload := DownloadPayload{URL: "https://tiktok.com/@u/video/2", UserID: 5, ChatID: 1}
	if err := client.EnqueueTikTok(payload); err != nil {
		t.Fatal(err)
	}

	if err := RecoverOrphanedActiveTasks(redisURL); err != nil {
		t.Fatal(err)
	}

	insp, err := NewInspector(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	defer insp.Close()

	pending, err := insp.ListPendingTasks(defaultQueueName)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected pending task to remain, got %d", len(pending))
	}
}

func TestRecoverOrphanedActiveTasks_removesOrphanWithStaleMetadata(t *testing.T) {
	redisURL, _ := testRedisURL(t)
	opt, err := RedisOpt(redisURL)
	if err != nil {
		t.Fatal(err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     opt.Addr,
		Password: opt.Password,
		DB:       opt.DB,
	})
	defer rdb.Close()

	ctx := context.Background()
	activeKey := fmt.Sprintf("asynq:{%s}:active", defaultQueue)
	orphanID := "stale-with-meta"
	taskKey := fmt.Sprintf("asynq:{%s}:t:%s", defaultQueue, orphanID)

	body, _ := json.Marshal(DownloadPayload{UserID: 1})
	task := asynq.NewTask(TypeDownload, body)
	if err := rdb.HSet(ctx, taskKey, map[string]any{
		"type":    "",
		"payload": string(task.Payload()),
	}).Err(); err != nil {
		t.Fatal(err)
	}
	if err := rdb.RPush(ctx, activeKey, orphanID).Err(); err != nil {
		t.Fatal(err)
	}

	if err := RecoverOrphanedActiveTasks(redisURL); err != nil {
		t.Fatal(err)
	}

	ids, err := rdb.LRange(ctx, activeKey, 0, -1).Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected orphan removed, active = %v", ids)
	}
}
