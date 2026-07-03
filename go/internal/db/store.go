package db

import (
	"context"
	"fmt"
	"log/slog"
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
	return fromDBLanguage(lang), nil
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
	`, userID, nullable(username), nullable(firstName), toDBLanguage(lang), time.Now().UTC())
	return err
}

func (s *Store) RecordDownload(ctx context.Context, userID, chatID int64, url, platform, status string, fileSizeMB float64, errMsg string) error {
	now := time.Now().UTC()
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO users (id, language, created_at) VALUES ($1, 'EN'::language, $2)
		ON CONFLICT (id) DO NOTHING
	`, userID, now); err != nil {
		slog.Warn("record download: ensure user failed", "user_id", userID, "err", err)
		return err
	}
	chatType := "group"
	if chatID == userID {
		chatType = "private"
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO chats (id, type, created_at) VALUES ($1, $2, $3)
		ON CONFLICT (id) DO NOTHING
	`, chatID, chatType, now); err != nil {
		slog.Warn("record download: ensure chat failed", "chat_id", chatID, "err", err)
		return err
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO downloads (user_id, chat_id, url, platform, status, file_size, error_message, created_at, completed_at)
		VALUES ($1, $2, $3, $4::platform, $5::downloadstatus, $6, $7, $8, $9)
	`, userID, chatID, url, toDBPlatform(platform), toDBDownloadStatus(status), int64(fileSizeMB*1024*1024), nullable(errMsg), now, now)
	if err != nil {
		slog.Warn("record download failed", "user_id", userID, "platform", platform, "status", status, "err", err)
	}
	return err
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

type UserSettings struct {
	YouTubeQuality string
	YouTubeRatio   string
}

func (s *Store) GetOrCreateUserSettings(ctx context.Context, userID int64) (UserSettings, error) {
	var settings UserSettings
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(youtube_quality, 'ask'), COALESCE(youtube_ratio, 'ask')
		FROM user_settings WHERE user_id = $1
	`, userID).Scan(&settings.YouTubeQuality, &settings.YouTubeRatio)
	if err == nil {
		return settings, nil
	}

	_, err = s.pool.Exec(ctx, `INSERT INTO users (id, language, created_at) VALUES ($1, 'EN'::language, $2) ON CONFLICT DO NOTHING`, userID, time.Now().UTC())
	if err != nil {
		return settings, err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO user_settings (user_id, youtube_quality, youtube_ratio)
		VALUES ($1, 'ask', 'ask')
		ON CONFLICT (user_id) DO NOTHING
	`, userID)
	if err != nil {
		return settings, err
	}
	return UserSettings{YouTubeQuality: "ask", YouTubeRatio: "ask"}, nil
}

func (s *Store) SetUserLanguage(ctx context.Context, userID int64, lang string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (id, language, created_at)
		VALUES ($1, $2::language, $3)
		ON CONFLICT (id) DO UPDATE SET language = EXCLUDED.language
	`, userID, toDBLanguage(lang), time.Now().UTC())
	return err
}

func (s *Store) SetYouTubeQuality(ctx context.Context, userID int64, quality string) error {
	return s.upsertSetting(ctx, userID, "youtube_quality", quality)
}

func (s *Store) SetYouTubeRatio(ctx context.Context, userID int64, ratio string) error {
	return s.upsertSetting(ctx, userID, "youtube_ratio", ratio)
}

func (s *Store) ResetUserSettings(ctx context.Context, userID int64) error {
	if _, err := s.pool.Exec(ctx, `UPDATE users SET language = 'EN'::language WHERE id = $1`, userID); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO user_settings (user_id, youtube_quality, youtube_ratio)
		VALUES ($1, 'ask', 'ask')
		ON CONFLICT (user_id) DO UPDATE SET youtube_quality = 'ask', youtube_ratio = 'ask'
	`, userID)
	return err
}

func (s *Store) upsertSetting(ctx context.Context, userID int64, column, value string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO users (id, language, created_at) VALUES ($1, 'EN'::language, $2) ON CONFLICT DO NOTHING`, userID, time.Now().UTC())
	if err != nil {
		return err
	}
	query := fmt.Sprintf(`
		INSERT INTO user_settings (user_id, %s) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET %s = EXCLUDED.%s
	`, column, column, column)
	_, err = s.pool.Exec(ctx, query, userID, value)
	return err
}

