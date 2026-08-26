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
	if err := store.CreateUser(ctx, userID, "tester", "Test", "ru", "saveinator"); err != nil {
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
	if err := store.CreateUser(ctx, userID, "alice", "Alice", "ru", "saveinator"); err != nil {
		t.Fatal(err)
	}
	lang, err := store.GetUserLanguage(ctx, userID, "saveinator")
	if err != nil {
		t.Fatal(err)
	}
	if lang != "ru" {
		t.Fatalf("language = %q, want ru", lang)
	}
	if err := store.SetUserLanguage(ctx, userID, "en", "saveinator"); err != nil {
		t.Fatal(err)
	}
	lang, err = store.GetUserLanguage(ctx, userID, "saveinator")
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
	seedUserAndChat(t, store, 2, 11)
	if err := store.RecordDownload(ctx, 1, 10, "https://youtu.be/x", "youtube", "completed", 0, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordDownload(ctx, 1, 10, "https://youtu.be/y", "youtube", "completed", 0, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordDownload(ctx, 2, 11, "https://open.spotify.com/track/x", "spotify", "completed", 0, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordDownload(ctx, 2, 11, "https://youtu.be/z", "youtube", "failed", 0, "err"); err != nil {
		t.Fatal(err)
	}

	stats, err := store.FetchUserStats(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalUsers < 2 {
		t.Fatalf("total users = %d", stats.TotalUsers)
	}
	if stats.UsersWithDownloads != 2 {
		t.Fatalf("users with downloads = %d, want 2", stats.UsersWithDownloads)
	}
	if stats.ReturningUsers != 1 {
		t.Fatalf("returning users = %d, want 1", stats.ReturningUsers)
	}
	if stats.DAU != 2 {
		t.Fatalf("dau = %d, want 2", stats.DAU)
	}
	if stats.DownloadsToday < 3 {
		t.Fatalf("downloads today = %d, want at least 3 completed downloads", stats.DownloadsToday)
	}
	if stats.Completed30d < 3 {
		t.Fatalf("completed 30d = %d", stats.Completed30d)
	}
	if stats.Failed30d < 1 {
		t.Fatalf("failed 30d = %d", stats.Failed30d)
	}
}

func TestIntegration_DashStats(t *testing.T) {
	store, cleanup := startTestStore(t)
	defer cleanup()
	ctx := context.Background()

	// Two users, one on the main bot and one on a fleet bot, with downloads
	// spread across platforms/statuses so every aggregation has data.
	seedUserAndChat(t, store, 101, 1001)
	if err := store.CreateUser(ctx, 102, "bob", "Bob", "en", "pinterest"); err != nil {
		t.Fatal(err)
	}
	_, err := store.pool.Exec(ctx, `INSERT INTO chats (id, type, created_at) VALUES (1002, 'private', now())`)
	if err != nil {
		t.Fatal(err)
	}
	dl := []struct {
		user, chat int64
		url, plat  string
		status     string
		bot        string
	}{
		{101, 1001, "https://youtu.be/a", "youtube", "completed", "saveinator"},
		{101, 1001, "https://youtu.be/b", "youtube", "completed", "saveinator"},
		{101, 1001, "https://tiktok.com/@x/v/1", "tiktok", "completed", "saveinator"},
		{102, 1002, "https://open.spotify.com/track/q", "spotify", "completed", "pinterest"},
		{102, 1002, "https://open.spotify.com/track/r", "spotify", "failed", "pinterest"},
	}
	for _, d := range dl {
		if err := store.RecordDownloadForBot(ctx, d.user, d.chat, d.url, d.plat, d.status, 1.0, "", d.bot); err != nil {
			t.Fatal(err)
		}
	}

	o, err := store.DashOverview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if o.TotalUsers < 2 {
		t.Fatalf("total users = %d, want >= 2", o.TotalUsers)
	}
	if o.DownloadsToday < 4 {
		t.Fatalf("downloads today = %d, want >= 4 completed", o.DownloadsToday)
	}
	if o.Completed30d < 4 || o.Failed30d < 1 {
		t.Fatalf("completed=%d failed=%d, want >=4 / >=1", o.Completed30d, o.Failed30d)
	}
	if o.DAU < 2 {
		t.Fatalf("dau = %d, want >= 2", o.DAU)
	}
	found := map[string]bool{}
	for _, p := range o.Platforms30d {
		found[p.Platform] = true
	}
	if !found["YOUTUBE"] || !found["TIKTOK"] || !found["SPOTIFY"] {
		t.Fatalf("platforms = %v, want YOUTUBE/TIKTOK/SPOTIFY", found)
	}
	botFound := map[string]bool{}
	for _, b := range o.Bots {
		botFound[b.Slug] = true
	}
	if !botFound["saveinator"] || !botFound["pinterest"] {
		t.Fatalf("bots = %v, want saveinator and pinterest", botFound)
	}
	if len(o.Languages) == 0 {
		t.Fatal("languages empty")
	}

	tl, err := store.DownloadTimeline(ctx, 14)
	if err != nil {
		t.Fatal(err)
	}
	if len(tl) == 0 {
		t.Fatal("timeline empty")
	}
	last := tl[len(tl)-1]
	if last.Total < 5 || last.Completed < 4 || last.Failed < 1 {
		t.Fatalf("last day point = %+v", last)
	}

	pl, err := store.PlatformBreakdown(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(pl) < 3 {
		t.Fatalf("platform breakdown rows = %d, want >= 3", len(pl))
	}

	users, err := store.UserTable(ctx, "downloads", "", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) < 2 {
		t.Fatalf("user table rows = %d, want >= 2", len(users))
	}
	if users[0].Downloads < users[1].Downloads {
		t.Fatalf("users not sorted by downloads: %+v", users)
	}
	byQ, err := store.UserTable(ctx, "newest", "bob", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(byQ) != 1 || byQ[0].ID != 102 {
		t.Fatalf("search 'bob' = %+v, want user 102", byQ)
	}
}
