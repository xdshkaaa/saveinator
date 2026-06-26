package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func Connect(ctx context.Context, databaseURL string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = 5
	cfg.MinConns = 1

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *Store) GetUserLanguage(ctx context.Context, userID int64) (string, error) {
	var lang string
	err := s.pool.QueryRow(ctx, `SELECT language::text FROM users WHERE id = $1`, userID).Scan(&lang)
	if err != nil {
		return "", err
	}
	return lang, nil
}

func (s *Store) UserExists(ctx context.Context, userID int64) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, userID).Scan(&exists)
	return exists, err
}

func (s *Store) CreateUser(ctx context.Context, userID int64, username, firstName, lang string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (id, username, first_name, language, created_at)
		VALUES ($1, $2, $3, $4::language, $5)
		ON CONFLICT (id) DO NOTHING
	`, userID, nullable(username), nullable(firstName), lang, time.Now().UTC())
	return err
}

func (s *Store) RecordDownload(ctx context.Context, userID, chatID int64, url, platform, status string, fileSizeMB float64, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO downloads (user_id, chat_id, url, platform, status, file_size, error_message, created_at, completed_at)
		VALUES ($1, $2, $3, $4::platform, $5::downloadstatus, $6, $7, $8, $9)
	`, userID, chatID, url, platform, status, int64(fileSizeMB*1024*1024), nullable(errMsg), time.Now().UTC(), time.Now().UTC())
	return err
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
