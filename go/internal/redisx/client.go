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

// NewWithRedis wraps an existing go-redis client (used in tests).
func NewWithRedis(rdb *redis.Client) *Client {
	return &Client{rdb: rdb}
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

func (c *Client) ForceReleaseUserLock(ctx context.Context, userID int64) error {
	key := fmt.Sprintf("%s:%d", lockPrefix, userID)
	return c.rdb.Del(ctx, key).Err()
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

func (c *Client) AllowURLDedup(ctx context.Context, urlHash string, window time.Duration) (bool, error) {
	if len(urlHash) < 12 {
		return true, nil
	}
	key := "dedup:" + urlHash[:12]
	ok, err := c.rdb.SetNX(ctx, key, "1", window).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}

type ActiveDownload struct {
	UserID   int64
	Scenario string
	Token    string
}

func (c *Client) GetActiveDownload(ctx context.Context, userID int64) (*ActiveDownload, error) {
	key := fmt.Sprintf("%s:%d", lockPrefix, userID)
	val, err := c.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(val, ":", 2)
	if len(parts) != 2 || parts[1] == "" {
		return nil, nil
	}
	return &ActiveDownload{UserID: userID, Scenario: parts[0], Token: parts[1]}, nil
}

const cancelPrefix = "download:cancel"

func (c *Client) SetDownloadCancelled(ctx context.Context, scenario string, userID int64, token string, ttl time.Duration) error {
	key := fmt.Sprintf("%s:%s:%d:%s", cancelPrefix, scenario, userID, token)
	return c.rdb.Set(ctx, key, "1", ttl).Err()
}

func (c *Client) IsDownloadCancelled(ctx context.Context, scenario string, userID int64, token string) (bool, error) {
	key := fmt.Sprintf("%s:%s:%d:%s", cancelPrefix, scenario, userID, token)
	n, err := c.rdb.Exists(ctx, key).Result()
	return n > 0, err
}

func (c *Client) TryAcquireReleaseLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	ok, err := c.rdb.SetNX(ctx, key, "1", ttl).Result()
	return ok, err
}

func (c *Client) ReleaseReleaseLock(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
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
