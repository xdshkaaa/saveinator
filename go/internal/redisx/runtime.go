package redisx

import (
	"context"

	"github.com/redis/go-redis/v9"
)

const runtimeSettingsKey = "saveinator:runtime_settings"

func (c *Client) RuntimeGet(ctx context.Context, field string) (string, bool, error) {
	val, err := c.rdb.HGet(ctx, runtimeSettingsKey, field).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return val, true, nil
}

func (c *Client) RuntimeSet(ctx context.Context, field, value string) error {
	return c.rdb.HSet(ctx, runtimeSettingsKey, field, value).Err()
}

func (c *Client) RuntimeDelete(ctx context.Context, field string) error {
	return c.rdb.HDel(ctx, runtimeSettingsKey, field).Err()
}

func (c *Client) RuntimeDeleteAll(ctx context.Context) error {
	return c.rdb.Del(ctx, runtimeSettingsKey).Err()
}
