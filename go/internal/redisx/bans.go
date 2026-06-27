package redisx

import (
	"context"
	"strconv"
)

const bannedUsersKey = "saveinator:banned_users"

func (c *Client) BanUser(ctx context.Context, userID int64) error {
	return c.rdb.SAdd(ctx, bannedUsersKey, strconv.FormatInt(userID, 10)).Err()
}

func (c *Client) UnbanUser(ctx context.Context, userID int64) (bool, error) {
	n, err := c.rdb.SRem(ctx, bannedUsersKey, strconv.FormatInt(userID, 10)).Result()
	return n > 0, err
}

func (c *Client) IsUserBanned(ctx context.Context, userID int64) (bool, error) {
	return c.rdb.SIsMember(ctx, bannedUsersKey, strconv.FormatInt(userID, 10)).Result()
}

func (c *Client) ListBannedUsers(ctx context.Context) ([]int64, error) {
	members, err := c.rdb.SMembers(ctx, bannedUsersKey).Result()
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(members))
	for _, m := range members {
		id, err := strconv.ParseInt(m, 10, 64)
		if err != nil {
			continue
		}
		out = append(out, id)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

func (c *Client) BannedCount(ctx context.Context) (int, error) {
	n, err := c.rdb.SCard(ctx, bannedUsersKey).Result()
	return int(n), err
}
