package queue

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

const defaultQueue = "default"

// RecoverOrphanedActiveTasks removes stale asynq active entries left behind after
// a crash or manual redis key deletion. Without this, the worker may dequeue tasks
// whose metadata is missing and silently skip user-visible progress updates.
func RecoverOrphanedActiveTasks(redisURL string) error {
	opt, err := RedisOpt(redisURL)
	if err != nil {
		return err
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     opt.Addr,
		Password: opt.Password,
		DB:       opt.DB,
	})
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	insp := asynq.NewInspector(opt)
	defer insp.Close()

	activeKey := fmt.Sprintf("asynq:{%s}:active", defaultQueue)
	leaseKey := fmt.Sprintf("asynq:{%s}:lease", defaultQueue)

	ids, err := rdb.LRange(ctx, activeKey, 0, -1).Result()
	if err != nil {
		return err
	}

	for _, id := range ids {
		if id == "" {
			continue
		}
		taskKey := fmt.Sprintf("asynq:{%s}:t:%s", defaultQueue, id)
		exists, err := rdb.Exists(ctx, taskKey).Result()
		if err != nil {
			return err
		}

		remove := exists == 0
		if !remove {
			info, err := insp.GetTaskInfo(defaultQueue, id)
			if err != nil || info == nil || info.Type == "" {
				remove = true
			}
		}

		if !remove {
			continue
		}

		if err := rdb.LRem(ctx, activeKey, 0, id).Err(); err != nil {
			return err
		}
		if err := rdb.ZRem(ctx, leaseKey, id).Err(); err != nil {
			return err
		}
		if exists > 0 {
			_ = rdb.Del(ctx, taskKey).Err()
		}
		slog.Warn("removed orphaned asynq active task", "id", id)
	}

	return nil
}
