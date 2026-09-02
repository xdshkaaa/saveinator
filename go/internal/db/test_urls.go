package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrDuplicateTestURL is returned when the URL is already on the checklist.
var ErrDuplicateTestURL = errors.New("test url already exists")

// Test URL run statuses (VARCHAR in test_urls.status).
const (
	TestStatusPending = "PENDING"
	TestStatusRunning = "RUNNING"
	TestStatusPassed  = "PASSED"
	TestStatusFailed  = "FAILED"
)

type TestURLRow struct {
	ID           int64      `json:"id"`
	URL          string     `json:"url"`
	Platform     string     `json:"platform"`
	Status       string     `json:"status"`
	ErrorMessage *string    `json:"error_message"`
	MediaType    *string    `json:"media_type"`
	FileSize     *int64     `json:"file_size"`
	DurationMS   *int       `json:"duration_ms"`
	CreatedAt    time.Time  `json:"created_at"`
	CheckedAt    *time.Time `json:"checked_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

const testURLColumns = `id, url, platform, status, error_message, media_type,
		file_size, duration_ms, created_at, checked_at, updated_at`

func scanTestURL(row pgx.Row) (TestURLRow, error) {
	var r TestURLRow
	err := row.Scan(&r.ID, &r.URL, &r.Platform, &r.Status, &r.ErrorMessage,
		&r.MediaType, &r.FileSize, &r.DurationMS, &r.CreatedAt, &r.CheckedAt, &r.UpdatedAt)
	return r, err
}

// ListTestURLs returns the whole checklist, oldest first.
func (s *Store) ListTestURLs(ctx context.Context) ([]TestURLRow, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+testURLColumns+` FROM test_urls ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TestURLRow
	for rows.Next() {
		r, err := scanTestURL(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CreateTestURL adds a URL to the checklist; duplicates are rejected.
func (s *Store) CreateTestURL(ctx context.Context, url, platform string) (TestURLRow, error) {
	r, err := scanTestURL(s.pool.QueryRow(ctx, `
		INSERT INTO test_urls (url, platform)
		VALUES ($1, $2)
		ON CONFLICT (url) DO NOTHING
		RETURNING `+testURLColumns+`
	`, url, platform))
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrDuplicateTestURL
	}
	return r, err
}

// DeleteTestURL removes a URL from the checklist.
func (s *Store) DeleteTestURL(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM test_urls WHERE id = $1`, id)
	return err
}

// ClaimNextTestURL atomically marks the next runnable row as RUNNING.
// Rows whose RUNNING state looks stale (>15 min, e.g. worker crashed mid-run)
// are picked up again. Returns nil when the checklist has nothing to run.
func (s *Store) ClaimNextTestURL(ctx context.Context) (*TestURLRow, error) {
	r, err := scanTestURL(s.pool.QueryRow(ctx, `
		UPDATE test_urls SET status = 'RUNNING', updated_at = now()
		WHERE id = (
			SELECT id FROM test_urls
			WHERE status = 'PENDING'
				OR (status = 'RUNNING' AND updated_at < now() - interval '15 minutes')
			ORDER BY id
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING `+testURLColumns+`
	`))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// FinishTestURL stores the outcome of one test run.
func (s *Store) FinishTestURL(ctx context.Context, id int64, status, errMsg, mediaType string, fileSize int64, durationMS int) error {
	var errMsgArg, mediaTypeArg any
	if errMsg != "" {
		errMsgArg = errMsg
	}
	if mediaType != "" {
		mediaTypeArg = mediaType
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE test_urls
		SET status = $2, error_message = $3, media_type = $4, file_size = $5,
			duration_ms = $6, checked_at = now(), updated_at = now()
		WHERE id = $1
	`, id, status, errMsgArg, mediaTypeArg, fileSize, durationMS)
	return err
}

// RequeueTestURLs puts finished rows back into the queue; with id != nil
// only that row is requeued. Returns how many rows were requeued.
func (s *Store) RequeueTestURLs(ctx context.Context, id *int64) (int64, error) {
	const reset = `status = 'PENDING', error_message = NULL, media_type = NULL,
			file_size = NULL, duration_ms = NULL, updated_at = now()`
	if id != nil {
		tag, err := s.pool.Exec(ctx,
			`UPDATE test_urls SET `+reset+` WHERE id = $1 AND status IN ('PASSED', 'FAILED')`, *id)
		if err != nil {
			return 0, err
		}
		return tag.RowsAffected(), nil
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE test_urls SET `+reset+` WHERE status IN ('PASSED', 'FAILED')`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
