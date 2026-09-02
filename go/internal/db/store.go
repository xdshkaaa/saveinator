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

// Ping verifies the database connection is alive.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// GetUserLanguage returns the user's language for a given bot, falling back
// to the global users.language when no per-bot row exists yet.
func (s *Store) GetUserLanguage(ctx context.Context, userID int64, botID string) (string, error) {
	var lang string
	err := s.pool.QueryRow(ctx, `SELECT language::text FROM user_bot_settings WHERE user_id = $1 AND bot_id = $2`, userID, botID).Scan(&lang)
	if err == nil {
		return fromDBLanguage(lang), nil
	}
	err = s.pool.QueryRow(ctx, `SELECT language::text FROM users WHERE id = $1`, userID).Scan(&lang)
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

func (s *Store) CreateUser(ctx context.Context, userID int64, username, firstName, lang, botID string) error {
	now := time.Now().UTC()
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO users (id, username, first_name, language, bot_id, created_at)
		VALUES ($1, $2, $3, $4::language, $5, $6)
		ON CONFLICT (id) DO NOTHING
	`, userID, nullable(username), nullable(firstName), toDBLanguage(lang), botID, now); err != nil {
		return err
	}
	return s.setUserBotLanguage(ctx, userID, botID, lang)
}

// RecordDownload records a download attributed to the default "saveinator"
// bot. Fleet bots (botkit) should use RecordDownloadForBot instead.
func (s *Store) RecordDownload(ctx context.Context, userID, chatID int64, url, platform, status string, fileSizeMB float64, errMsg string) error {
	return s.RecordDownloadForBot(ctx, userID, chatID, url, platform, status, fileSizeMB, errMsg, "saveinator")
}

func (s *Store) RecordDownloadForBot(ctx context.Context, userID, chatID int64, url, platform, status string, fileSizeMB float64, errMsg, botID string) error {
	now := time.Now().UTC()
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO users (id, language, bot_id, created_at) VALUES ($1, 'EN'::language, $2, $3)
		ON CONFLICT (id) DO NOTHING
	`, userID, botID, now); err != nil {
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
		INSERT INTO downloads (user_id, chat_id, url, platform, status, bot_id, file_size, error_message, created_at, completed_at)
		VALUES ($1, $2, $3, $4::platform, $5::downloadstatus, $6, $7, $8, $9, $10)
	`, userID, chatID, url, toDBPlatform(platform), toDBDownloadStatus(status), botID, int64(fileSizeMB*1024*1024), nullable(errMsg), now, now)
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
	NoWatermark    bool
}

func (s *Store) GetOrCreateUserSettings(ctx context.Context, userID int64) (UserSettings, error) {
	var settings UserSettings
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(youtube_quality, 'ask'), COALESCE(youtube_ratio, 'ask'), COALESCE(no_watermark, FALSE)
		FROM user_settings WHERE user_id = $1
	`, userID).Scan(&settings.YouTubeQuality, &settings.YouTubeRatio, &settings.NoWatermark)
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

// SetUserLanguage sets the user's language for a given bot, and also updates
// the global fallback in users.language.
func (s *Store) SetUserLanguage(ctx context.Context, userID int64, lang, botID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (id, language, bot_id, created_at)
		VALUES ($1, $2::language, $3, $4)
		ON CONFLICT (id) DO UPDATE SET language = EXCLUDED.language
	`, userID, toDBLanguage(lang), botID, time.Now().UTC())
	if err != nil {
		return err
	}
	return s.setUserBotLanguage(ctx, userID, botID, lang)
}

func (s *Store) setUserBotLanguage(ctx context.Context, userID int64, botID, lang string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO user_bot_settings (user_id, bot_id, language, created_at)
		VALUES ($1, $2, $3::language, $4)
		ON CONFLICT (user_id, bot_id) DO UPDATE SET language = EXCLUDED.language
	`, userID, botID, toDBLanguage(lang), time.Now().UTC())
	return err
}

func (s *Store) SetYouTubeQuality(ctx context.Context, userID int64, quality string) error {
	return s.upsertSetting(ctx, userID, "youtube_quality", quality)
}

func (s *Store) SetYouTubeRatio(ctx context.Context, userID int64, ratio string) error {
	return s.upsertSetting(ctx, userID, "youtube_ratio", ratio)
}

// SetNoWatermark toggles the "hide the via-footer" preference. It only has an
// effect for users entitled to it (a Stars purchase or the admin).
func (s *Store) SetNoWatermark(ctx context.Context, userID int64, enabled bool) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO users (id, language, created_at) VALUES ($1, 'EN'::language, $2) ON CONFLICT DO NOTHING`, userID, time.Now().UTC())
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO user_settings (user_id, no_watermark) VALUES ($1, $2::boolean)
		ON CONFLICT (user_id) DO UPDATE SET no_watermark = EXCLUDED.no_watermark
	`, userID, enabled)
	return err
}

// HasPurchase reports whether the user has already bought the given product.
func (s *Store) HasPurchase(ctx context.Context, userID int64, product string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM purchases WHERE user_id = $1 AND product = $2)
	`, userID, product).Scan(&exists)
	return exists, err
}

// RecordPurchase stores a completed payment. The unique charge id makes it
// idempotent: redelivered successful_payment updates insert nothing. It
// returns whether this call recorded a new purchase.
func (s *Store) RecordPurchase(ctx context.Context, userID int64, product string, stars int, currency, chargeID string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO purchases (user_id, product, stars_amount, currency, telegram_payment_charge_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (telegram_payment_charge_id) DO NOTHING
	`, userID, product, stars, currency, chargeID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) ResetUserSettings(ctx context.Context, userID int64) error {
	if _, err := s.pool.Exec(ctx, `UPDATE users SET language = 'EN'::language WHERE id = $1`, userID); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO user_settings (user_id, youtube_quality, youtube_ratio, no_watermark)
		VALUES ($1, 'ask', 'ask', FALSE)
		ON CONFLICT (user_id) DO UPDATE SET youtube_quality = 'ask', youtube_ratio = 'ask', no_watermark = FALSE
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
