package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Broadcast struct {
	ID               int
	AdminID          int64
	Text             string
	Audience         string
	Status           string
	TotalRecipients  int
	SentCount        int
	FailedCount      int
	BlockedCount     int
	CreatedAt        time.Time
	StartedAt        *time.Time
	FinishedAt       *time.Time
}

func (s *Store) CreateBroadcast(ctx context.Context, adminID int64, text string) (int, error) {
	var id int
	err := s.pool.QueryRow(ctx, `
		INSERT INTO broadcasts (admin_id, text, audience, status, created_at)
		VALUES ($1, $2, 'ALL', 'DRAFT', $3)
		RETURNING id
	`, adminID, text, time.Now().UTC()).Scan(&id)
	return id, err
}

func (s *Store) GetBroadcast(ctx context.Context, id int) (*Broadcast, error) {
	var b Broadcast
	var started, finished *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, admin_id, text, audience::text, status::text,
			total_recipients, sent_count, failed_count, blocked_count,
			created_at, started_at, finished_at
		FROM broadcasts WHERE id = $1
	`, id).Scan(
		&b.ID, &b.AdminID, &b.Text, &b.Audience, &b.Status,
		&b.TotalRecipients, &b.SentCount, &b.FailedCount, &b.BlockedCount,
		&b.CreatedAt, &started, &finished,
	)
	if err != nil {
		return nil, err
	}
	b.StartedAt = started
	b.FinishedAt = finished
	return &b, nil
}

func (s *Store) UpdateBroadcastText(ctx context.Context, id int, text string) error {
	_, err := s.pool.Exec(ctx, `UPDATE broadcasts SET text = $2 WHERE id = $1`, id, text)
	return err
}

func (s *Store) UpdateBroadcastStatus(ctx context.Context, id int, status string, total int) error {
	now := time.Now().UTC()
	switch status {
	case "QUEUED":
		_, err := s.pool.Exec(ctx, `
			UPDATE broadcasts SET status = $2::broadcaststatus, total_recipients = $3 WHERE id = $1
		`, id, status, total)
		return err
	case "RUNNING":
		_, err := s.pool.Exec(ctx, `
			UPDATE broadcasts SET status = $2::broadcaststatus, started_at = $3, total_recipients = $4 WHERE id = $1
		`, id, status, now, total)
		return err
	case "COMPLETED":
		_, err := s.pool.Exec(ctx, `
			UPDATE broadcasts SET status = $2::broadcaststatus, finished_at = $3 WHERE id = $1
		`, id, status, now)
		return err
	case "FAILED", "CANCELLED":
		_, err := s.pool.Exec(ctx, `
			UPDATE broadcasts SET status = $2::broadcaststatus, finished_at = $3 WHERE id = $1
		`, id, status, now)
		return err
	default:
		_, err := s.pool.Exec(ctx, `UPDATE broadcasts SET status = $2::broadcaststatus WHERE id = $1`, id, status)
		return err
	}
}

func (s *Store) UpdateBroadcastProgress(ctx context.Context, id, sent, failed, blocked int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE broadcasts SET sent_count = $2, failed_count = $3, blocked_count = $4 WHERE id = $1
	`, id, sent, failed, blocked)
	return err
}

func (s *Store) CompleteBroadcast(ctx context.Context, id, sent, failed, blocked int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE broadcasts
		SET status = 'COMPLETED'::broadcaststatus, sent_count = $2, failed_count = $3,
			blocked_count = $4, finished_at = $5
		WHERE id = $1
	`, id, sent, failed, blocked, time.Now().UTC())
	return err
}

func (s *Store) FailBroadcast(ctx context.Context, id int) error {
	return s.UpdateBroadcastStatus(ctx, id, "FAILED", 0)
}

func (s *Store) ListBroadcasts(ctx context.Context, limit int) ([]Broadcast, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, admin_id, text, audience::text, status::text,
			total_recipients, sent_count, failed_count, blocked_count,
			created_at, started_at, finished_at
		FROM broadcasts ORDER BY created_at DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Broadcast
	for rows.Next() {
		var b Broadcast
		var started, finished *time.Time
		if err := rows.Scan(
			&b.ID, &b.AdminID, &b.Text, &b.Audience, &b.Status,
			&b.TotalRecipients, &b.SentCount, &b.FailedCount, &b.BlockedCount,
			&b.CreatedAt, &started, &finished,
		); err != nil {
			return nil, err
		}
		b.StartedAt = started
		b.FinishedAt = finished
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) GetActiveBroadcast(ctx context.Context) (*Broadcast, error) {
	var b Broadcast
	var started, finished *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, admin_id, text, audience::text, status::text,
			total_recipients, sent_count, failed_count, blocked_count,
			created_at, started_at, finished_at
		FROM broadcasts
		WHERE status IN ('QUEUED', 'RUNNING')
		ORDER BY created_at DESC LIMIT 1
	`).Scan(
		&b.ID, &b.AdminID, &b.Text, &b.Audience, &b.Status,
		&b.TotalRecipients, &b.SentCount, &b.FailedCount, &b.BlockedCount,
		&b.CreatedAt, &started, &finished,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.StartedAt = started
	b.FinishedAt = finished
	return &b, nil
}

func (s *Store) CountBroadcastRecipients(ctx context.Context, audience string) (int, error) {
	query := `SELECT COUNT(*) FROM users`
	switch audience {
	case "ru":
		query += ` WHERE language = 'ru'::language`
	case "en":
		query += ` WHERE language = 'en'::language`
	case "active":
		query = `
			SELECT COUNT(DISTINCT user_id) FROM downloads
			WHERE created_at >= NOW() - INTERVAL '30 days'
		`
	}
	var n int
	err := s.pool.QueryRow(ctx, query).Scan(&n)
	return n, err
}

func (s *Store) BroadcastRecipientIDs(ctx context.Context, audience string) ([]int64, error) {
	var query string
	switch audience {
	case "ru":
		query = `SELECT id FROM users WHERE language = 'ru'::language`
	case "en":
		query = `SELECT id FROM users WHERE language = 'en'::language`
	case "active":
		query = `
			SELECT DISTINCT user_id FROM downloads
			WHERE created_at >= NOW() - INTERVAL '30 days'
		`
	default:
		query = `SELECT id FROM users`
	}
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) SaveBroadcastDelivery(ctx context.Context, broadcastID int, userID int64, status, errMsg string) error {
	var exists bool
	_ = s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM broadcast_deliveries WHERE broadcast_id = $1 AND user_id = $2)
	`, broadcastID, userID).Scan(&exists)

	if exists {
		sentAt := any(nil)
		if status == "SENT" {
			sentAt = time.Now().UTC()
		}
		_, err := s.pool.Exec(ctx, `
			UPDATE broadcast_deliveries
			SET status = $3::broadcastdeliverystatus, error_message = $4, sent_at = COALESCE($5, sent_at)
			WHERE broadcast_id = $1 AND user_id = $2
		`, broadcastID, userID, status, nullable(errMsg), sentAt)
		return err
	}

	sentAt := (*time.Time)(nil)
	if status == "SENT" {
		t := time.Now().UTC()
		sentAt = &t
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO broadcast_deliveries (broadcast_id, user_id, status, error_message, sent_at)
		VALUES ($1, $2, $3::broadcastdeliverystatus, $4, $5)
	`, broadcastID, userID, status, nullable(errMsg), sentAt)
	return err
}
