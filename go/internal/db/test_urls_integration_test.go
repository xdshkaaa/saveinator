package db

import (
	"context"
	"errors"
	"testing"
)

func TestIntegration_TestURLLifecycle(t *testing.T) {
	store, cleanup := startTestStore(t)
	defer cleanup()
	ctx := context.Background()

	first, err := store.CreateTestURL(ctx, "https://youtu.be/aaaaaaaaaaa", "youtube")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == 0 || first.Status != TestStatusPending {
		t.Fatalf("created row = %+v", first)
	}
	if _, err := store.CreateTestURL(ctx, "https://youtu.be/aaaaaaaaaaa", "youtube"); !errors.Is(err, ErrDuplicateTestURL) {
		t.Fatalf("duplicate create err = %v, want ErrDuplicateTestURL", err)
	}

	second, err := store.CreateTestURL(ctx, "https://pin.it/xyz123", "pinterest")
	if err != nil {
		t.Fatal(err)
	}

	rows, err := store.ListTestURLs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID != first.ID || rows[1].ID != second.ID {
		t.Fatalf("list = %+v", rows)
	}

	// Claim drains in id order, then reports empty.
	claim, err := store.ClaimNextTestURL(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.ID != first.ID || claim.Status != TestStatusRunning {
		t.Fatalf("first claim = %+v", claim)
	}
	claim, err = store.ClaimNextTestURL(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.ID != second.ID {
		t.Fatalf("second claim = %+v", claim)
	}
	if claim, err = store.ClaimNextTestURL(ctx); err != nil || claim != nil {
		t.Fatalf("third claim = %+v err = %v, want nil", claim, err)
	}

	if err := store.FinishTestURL(ctx, first.ID, TestStatusFailed, "yt-dlp exploded", "", 0, 1234); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishTestURL(ctx, second.ID, TestStatusPassed, "", "video", 54321, 900); err != nil {
		t.Fatal(err)
	}

	rows, err = store.ListTestURLs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Status != TestStatusFailed || rows[0].ErrorMessage == nil || *rows[0].ErrorMessage != "yt-dlp exploded" || rows[0].DurationMS == nil || *rows[0].DurationMS != 1234 {
		t.Fatalf("failed row = %+v", rows[0])
	}
	if rows[1].Status != TestStatusPassed || rows[1].MediaType == nil || *rows[1].MediaType != "video" || rows[1].FileSize == nil || *rows[1].FileSize != 54321 || rows[1].CheckedAt == nil {
		t.Fatalf("passed row = %+v", rows[1])
	}

	n, err := store.RequeueTestURLs(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("requeued = %d, want 2", n)
	}
	rows, err = store.ListTestURLs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Status != TestStatusPending || r.ErrorMessage != nil {
			t.Fatalf("requeued row = %+v", r)
		}
	}

	// RUNNING rows are not requeueable; finished ones are.
	if _, err := store.ClaimNextTestURL(ctx); err != nil {
		t.Fatal(err)
	}
	if n, err = store.RequeueTestURLs(ctx, &first.ID); err != nil || n != 0 {
		t.Fatalf("requeue running = %d, %v; want 0", n, err)
	}
	if n, err = store.RequeueTestURLs(ctx, &second.ID); err != nil || n != 1 {
		t.Fatalf("requeue finished = %d, %v; want 1", n, err)
	}

	if err := store.DeleteTestURL(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	if rows, err = store.ListTestURLs(ctx); err != nil || len(rows) != 1 || rows[0].ID != first.ID {
		t.Fatalf("after delete rows = %+v err = %v", rows, err)
	}
}

func TestIntegration_TestURLStaleRunningReclaimed(t *testing.T) {
	store, cleanup := startTestStore(t)
	defer cleanup()
	ctx := context.Background()

	row, err := store.CreateTestURL(ctx, "https://youtu.be/bbbbbbbbbbb", "youtube")
	if err != nil {
		t.Fatal(err)
	}
	claim, err := store.ClaimNextTestURL(ctx)
	if err != nil || claim == nil || claim.ID != row.ID {
		t.Fatalf("claim = %+v err = %v", claim, err)
	}

	// A crashed worker leaves the row RUNNING forever; the claim query must
	// pick it back up once its RUNNING state looks stale.
	if _, err := store.pool.Exec(ctx,
		`UPDATE test_urls SET updated_at = now() - interval '20 minutes' WHERE id = $1`, row.ID); err != nil {
		t.Fatal(err)
	}
	claim, err = store.ClaimNextTestURL(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.ID != row.ID || claim.Status != TestStatusRunning {
		t.Fatalf("stale reclaim = %+v, want row %d RUNNING", claim, row.ID)
	}
}
