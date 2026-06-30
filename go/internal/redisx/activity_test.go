package redisx

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestTouchActiveUser_andCount(t *testing.T) {
	t.Parallel()
	c, _ := testClient(t)
	ctx := context.Background()

	if err := c.TouchActiveUser(ctx, 1001); err != nil {
		t.Fatal(err)
	}
	n, err := c.CountActiveUsers(ctx, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
}

func TestCountActiveUsers_expiresOldEntries(t *testing.T) {
	t.Parallel()
	c, _ := testClient(t)
	ctx := context.Background()

	now := float64(time.Now().UnixNano()) / 1e9
	old := now - 31*60
	if err := c.Raw().ZAdd(ctx, activeUsersKey, redis.Z{Score: old, Member: "2002"}).Err(); err != nil {
		t.Fatal(err)
	}

	n, err := c.CountActiveUsers(ctx, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("count = %d, want 0 after window", n)
	}
}

func TestTouchActiveUser_ignoresZero(t *testing.T) {
	t.Parallel()
	c, _ := testClient(t)
	ctx := context.Background()

	if err := c.TouchActiveUser(ctx, 0); err != nil {
		t.Fatal(err)
	}
	n, err := c.CountActiveUsers(ctx, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("count = %d, want 0", n)
	}
}
