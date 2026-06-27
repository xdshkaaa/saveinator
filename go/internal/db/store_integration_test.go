package db

import (
	"context"
	_ "embed"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

//go:embed testdata/schema.sql
var testSchemaSQL string

func startTestStore(t *testing.T) (*Store, func()) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping DB integration test in short mode")
	}

	ctx := context.Background()
	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("saveinator"),
		postgres.WithUsername("saveinator"),
		postgres.WithPassword("password"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		t.Skipf("postgres container unavailable: %v", err)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("connection string: %v", err)
	}

	cfg, err := pgx.ParseConfig(connStr)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("parse config: %v", err)
	}
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("connect: %v", err)
	}
	if _, err := conn.Exec(ctx, testSchemaSQL); err != nil {
		conn.Close(ctx)
		_ = container.Terminate(ctx)
		t.Fatalf("apply schema: %v", err)
	}
	conn.Close(ctx)

	store, err := Connect(ctx, connStr)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("store connect: %v", err)
	}

	cleanup := func() {
		store.Close()
		_ = container.Terminate(ctx)
	}
	return store, cleanup
}

func seedUserAndChat(t *testing.T, store *Store, userID, chatID int64) {
	t.Helper()
	ctx := context.Background()
	if err := store.CreateUser(ctx, userID, "tester", "Test", "ru"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	_, err := store.pool.Exec(ctx, `INSERT INTO chats (id, type, created_at) VALUES ($1, 'private', $2)`, chatID, time.Now().UTC())
	if err != nil {
		t.Fatalf("create chat: %v", err)
	}
}

func TestIntegration_CreateUserLanguageRoundtrip(t *testing.T) {
	store, cleanup := startTestStore(t)
	defer cleanup()
	ctx := context.Background()

	const userID int64 = 1001
	if err := store.CreateUser(ctx, userID, "alice", "Alice", "ru"); err != nil {
		t.Fatal(err)
	}
	lang, err := store.GetUserLanguage(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if lang != "ru" {
		t.Fatalf("language = %q, want ru", lang)
	}
	if err := store.SetUserLanguage(ctx, userID, "en"); err != nil {
		t.Fatal(err)
	}
	lang, err = store.GetUserLanguage(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if lang != "en" {
		t.Fatalf("language = %q, want en", lang)
	}
}

func TestIntegration_GetOrCreateUserSettings(t *testing.T) {
	store, cleanup := startTestStore(t)
	defer cleanup()
	ctx := context.Background()

	const userID int64 = 2002
	settings, err := store.GetOrCreateUserSettings(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if settings.YouTubeQuality != "ask" || settings.YouTubeRatio != "ask" {
		t.Fatalf("defaults = %+v", settings)
	}
	if err := store.SetYouTubeQuality(ctx, userID, "720"); err != nil {
		t.Fatal(err)
	}
	settings, err = store.GetOrCreateUserSettings(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if settings.YouTubeQuality != "720" {
		t.Fatalf("quality = %q", settings.YouTubeQuality)
	}
}

func TestIntegration_RecordDownload(t *testing.T) {
	store, cleanup := startTestStore(t)
	defer cleanup()
	ctx := context.Background()

	const userID int64 = 3003
	const chatID int64 = 4004
	seedUserAndChat(t, store, userID, chatID)

	if err := store.RecordDownload(ctx, userID, chatID, "https://tiktok.com/v/1", "tiktok", "completed", 1.5, ""); err != nil {
		t.Fatal(err)
	}

	var status, platform string
	err := store.pool.QueryRow(ctx, `
		SELECT status::text, platform::text FROM downloads WHERE user_id = $1
	`, userID).Scan(&status, &platform)
	if err != nil {
		t.Fatal(err)
	}
	if status != "COMPLETED" || platform != "TIKTOK" {
		t.Fatalf("status=%q platform=%q", status, platform)
	}
}

func TestIntegration_CreateBroadcast(t *testing.T) {
	store, cleanup := startTestStore(t)
	defer cleanup()
	ctx := context.Background()

	id, err := store.CreateBroadcast(ctx, 1, "hello world")
	if err != nil {
		t.Fatal(err)
	}
	bc, err := store.GetBroadcast(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if bc.Status != "DRAFT" || bc.Text != "hello world" {
		t.Fatalf("broadcast = %+v", bc)
	}
	if err := store.UpdateBroadcastStatus(ctx, id, "QUEUED", 10); err != nil {
		t.Fatal(err)
	}
	bc, err = store.GetBroadcast(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if bc.Status != "QUEUED" || bc.TotalRecipients != 10 {
		t.Fatalf("queued broadcast = %+v", bc)
	}
}

func TestIntegration_FetchUserStats(t *testing.T) {
	store, cleanup := startTestStore(t)
	defer cleanup()
	ctx := context.Background()

	seedUserAndChat(t, store, 1, 10)
	if err := store.RecordDownload(ctx, 1, 10, "https://youtu.be/x", "youtube", "completed", 0, ""); err != nil {
		t.Fatal(err)
	}

	stats, err := store.FetchUserStats(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalUsers < 1 {
		t.Fatalf("stats = %+v", stats)
	}
}
