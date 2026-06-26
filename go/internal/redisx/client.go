package redisx

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type Client struct {
	rdb *redis.Client
}

func Connect(redisURL string) (*Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	rdb := redis.NewClient(opts)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return &Client{rdb: rdb}, nil
}

func (c *Client) Close() error {
	return c.rdb.Close()
}

func (c *Client) Raw() *redis.Client {
	return c.rdb
}

const lockPrefix = "user_busy"

var releaseScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
end
return 0
`)

func (c *Client) AcquireUserLock(ctx context.Context, userID int64, scenario string, ttl time.Duration) (string, bool, error) {
	token := uuid.NewString()
	key := fmt.Sprintf("%s:%d", lockPrefix, userID)
	value := scenario + ":" + token
	ok, err := c.rdb.SetNX(ctx, key, value, ttl).Result()
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}
	return token, true, nil
}

func (c *Client) ReleaseUserLock(ctx context.Context, userID int64, scenario, token string) error {
	key := fmt.Sprintf("%s:%d", lockPrefix, userID)
	value := scenario + ":" + token
	_, err := releaseScript.Run(ctx, c.rdb, []string{key}, value).Result()
	return err
}

func (c *Client) AllowRateLimit(ctx context.Context, scope string, id int64, limit int, window time.Duration) (bool, error) {
	now := float64(time.Now().UnixNano()) / 1e9
	key := fmt.Sprintf("ratelimit:%s:%d", scope, id)
	pipe := c.rdb.Pipeline()
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%f", now-float64(window.Seconds())))
	countCmd := pipe.ZCard(ctx, key)
	pipe.ZAdd(ctx, key, redis.Z{Score: now, Member: fmt.Sprintf("%f", now)})
	pipe.Expire(ctx, key, window+10*time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}
	return countCmd.Val() < int64(limit), nil
}

func ParseRedisAddr(redisURL string) (string, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return "", err
	}
	addr := opts.Addr
	if !strings.Contains(addr, ":") {
		addr += ":6379"
	}
	return addr, nil
}
