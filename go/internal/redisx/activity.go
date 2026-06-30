package redisx

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const activeUsersKey = "active:users"

func (c *Client) TouchActiveUser(ctx context.Context, userID int64) error {
	if userID == 0 {
		return nil
	}
	now := float64(time.Now().UnixNano()) / 1e9
	pipe := c.rdb.Pipeline()
	pipe.ZAdd(ctx, activeUsersKey, redis.Z{Score: now, Member: strconv.FormatInt(userID, 10)})
	pipe.Expire(ctx, activeUsersKey, 35*time.Minute)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *Client) CountActiveUsers(ctx context.Context, window time.Duration) (int, error) {
	now := float64(time.Now().UnixNano()) / 1e9
	min := fmt.Sprintf("%f", now-float64(window.Seconds()))
	pipe := c.rdb.Pipeline()
	pipe.ZRemRangeByScore(ctx, activeUsersKey, "0", min)
	countCmd := pipe.ZCount(ctx, activeUsersKey, min, "+inf")
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return int(countCmd.Val()), nil
}
