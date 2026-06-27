package redisx

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func testClient(t *testing.T) (*Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewWithRedis(rdb), mr
}

func TestAcquireUserLock_success(t *testing.T) {
	t.Parallel()
	c, _ := testClient(t)
	ctx := context.Background()

	token, ok, err := c.AcquireUserLock(ctx, 42, "tiktok", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || token == "" {
		t.Fatal("expected lock acquired")
	}
}

func TestAcquireUserLock_conflict(t *testing.T) {
	t.Parallel()
	c, _ := testClient(t)
	ctx := context.Background()

	if _, ok, err := c.AcquireUserLock(ctx, 42, "tiktok", time.Minute); err != nil || !ok {
		t.Fatal("first lock should succeed")
	}
	_, ok, err := c.AcquireUserLock(ctx, 42, "pinterest", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("second lock should fail")
	}
}

func TestReleaseUserLock_requiresMatchingToken(t *testing.T) {
	t.Parallel()
	c, _ := testClient(t)
	ctx := context.Background()

	token, ok, err := c.AcquireUserLock(ctx, 7, "tiktok", time.Minute)
	if err != nil || !ok {
		t.Fatal("lock acquire failed")
	}
	if err := c.ReleaseUserLock(ctx, 7, "tiktok", "wrong"); err != nil {
		t.Fatal(err)
	}
	active, err := c.GetActiveDownload(ctx, 7)
	if err != nil || active == nil {
		t.Fatal("lock should remain after wrong token release")
	}
	if err := c.ReleaseUserLock(ctx, 7, "tiktok", token); err != nil {
		t.Fatal(err)
	}
	active, err = c.GetActiveDownload(ctx, 7)
	if err != nil || active != nil {
		t.Fatal("lock should be released")
	}
}

func TestForceReleaseUserLock(t *testing.T) {
	t.Parallel()
	c, _ := testClient(t)
	ctx := context.Background()

	if _, ok, err := c.AcquireUserLock(ctx, 9, "tiktok", time.Minute); err != nil || !ok {
		t.Fatal("lock acquire failed")
	}
	if err := c.ForceReleaseUserLock(ctx, 9); err != nil {
		t.Fatal(err)
	}
	active, err := c.GetActiveDownload(ctx, 9)
	if err != nil || active != nil {
		t.Fatal("lock should be force-released")
	}
}

func TestAllowRateLimit_window(t *testing.T) {
	t.Parallel()
	c, _ := testClient(t)
	ctx := context.Background()

	const limit = 3
	window := time.Minute
	for i := 0; i < limit; i++ {
		ok, err := c.AllowRateLimit(ctx, "user", 100, limit, window)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	ok, err := c.AllowRateLimit(ctx, "user", 100, limit, window)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("request over limit should be denied")
	}
}

func TestAllowURLDedup(t *testing.T) {
	t.Parallel()
	c, _ := testClient(t)
	ctx := context.Background()
	hash := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	window := time.Minute

	ok, err := c.AllowURLDedup(ctx, hash, window)
	if err != nil || !ok {
		t.Fatal("first URL should be allowed")
	}
	ok, err = c.AllowURLDedup(ctx, hash, window)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("duplicate URL should be blocked")
	}
}

func TestDownloadCancelledLifecycle(t *testing.T) {
	t.Parallel()
	c, _ := testClient(t)
	ctx := context.Background()

	cancelled, err := c.IsDownloadCancelled(ctx, "tiktok", 1, "tok")
	if err != nil {
		t.Fatal(err)
	}
	if cancelled {
		t.Fatal("should not be cancelled initially")
	}
	if err := c.SetDownloadCancelled(ctx, "tiktok", 1, "tok", time.Minute); err != nil {
		t.Fatal(err)
	}
	cancelled, err = c.IsDownloadCancelled(ctx, "tiktok", 1, "tok")
	if err != nil || !cancelled {
		t.Fatal("should be cancelled after set")
	}
}

func TestGetActiveDownload_parsesLock(t *testing.T) {
	t.Parallel()
	c, _ := testClient(t)
	ctx := context.Background()

	token, ok, err := c.AcquireUserLock(ctx, 55, "pinterest", time.Minute)
	if err != nil || !ok {
		t.Fatal("lock acquire failed")
	}
	active, err := c.GetActiveDownload(ctx, 55)
	if err != nil {
		t.Fatal(err)
	}
	if active == nil || active.Scenario != "pinterest" || active.Token != token {
		t.Fatalf("active = %+v, want pinterest:%s", active, token)
	}
}

func TestRuntimeGetSet(t *testing.T) {
	t.Parallel()
	c, _ := testClient(t)
	ctx := context.Background()

	if _, ok, err := c.RuntimeGet(ctx, "spotify.enabled"); err != nil || ok {
		t.Fatal("expected missing key")
	}
	if err := c.RuntimeSet(ctx, "spotify.enabled", "0"); err != nil {
		t.Fatal(err)
	}
	val, ok, err := c.RuntimeGet(ctx, "spotify.enabled")
	if err != nil || !ok || val != "0" {
		t.Fatalf("RuntimeGet() = (%q, %v, %v)", val, ok, err)
	}
	if err := c.RuntimeDelete(ctx, "spotify.enabled"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := c.RuntimeGet(ctx, "spotify.enabled"); err != nil || ok {
		t.Fatal("expected key deleted")
	}
}
