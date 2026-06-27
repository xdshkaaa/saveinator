package runtime

import (
	"context"
	"testing"

	"saveinator/internal/config"
	"saveinator/internal/redisx"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func testStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	client := redisx.NewWithRedis(rdb)
	cfg := &config.Settings{
		SpotifyEnabled:                  true,
		InstagramDownloadTimeoutSeconds: 120,
		SendVideoLimitMB:                50,
	}
	return NewStore(client, cfg), mr
}

func TestRuntimeStorePlatformEnabled(t *testing.T) {
	t.Parallel()
	store, _ := testStore(t)
	ctx := context.Background()

	if !store.PlatformEnabled(ctx, "spotify") {
		t.Fatal("expected spotify enabled from config default")
	}
	if err := store.SetValue(ctx, "spotify.enabled", false); err != nil {
		t.Fatal(err)
	}
	if store.PlatformEnabled(ctx, "spotify") {
		t.Fatal("expected spotify disabled from redis override")
	}
}

func TestRuntimeStorePlatformTimeoutSec(t *testing.T) {
	t.Parallel()
	store, _ := testStore(t)
	ctx := context.Background()

	if got := store.PlatformTimeoutSec(ctx, "instagram"); got != 120 {
		t.Fatalf("timeout = %d, want 120", got)
	}
	if err := store.SetValue(ctx, "instagram.timeout_sec", 90); err != nil {
		t.Fatal(err)
	}
	if got := store.PlatformTimeoutSec(ctx, "instagram"); got != 90 {
		t.Fatalf("timeout = %d, want 90", got)
	}
}

func TestRuntimeStorePlatformMaxFileMB(t *testing.T) {
	t.Parallel()
	store, _ := testStore(t)
	ctx := context.Background()

	if got := store.PlatformMaxFileMB(ctx, "tiktok"); got != 50 {
		t.Fatalf("max file = %d, want 50", got)
	}
	if err := store.SetValue(ctx, "tiktok.max_file_mb", 30); err != nil {
		t.Fatal(err)
	}
	if got := store.PlatformMaxFileMB(ctx, "tiktok"); got != 30 {
		t.Fatalf("max file = %d, want 30", got)
	}
}
