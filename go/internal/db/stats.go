package db

import (
	"context"
	"time"
)

type UserStats struct {
	TotalUsers         int
	NewToday           int
	NewYesterday       int
	New7d              int
	New30d             int
	DAU                int
	WAU                int
	MAU                int
	UsersWithDownloads int
	ReturningUsers     int
	LanguageEN         int
	LanguageRU         int
	TopPlatforms7d     []PlatformCount
	BannedCount        int
}

type PlatformCount struct {
	Platform string
	Count    int
}

func (s *Store) FetchUserStats(ctx context.Context, bannedCount int) (UserStats, error) {
	var stats UserStats
	stats.BannedCount = bannedCount
	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	yesterdayStart := todayStart.Add(-24 * time.Hour)

	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&stats.TotalUsers); err != nil {
		return stats, err
	}
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE created_at >= $1`, todayStart).Scan(&stats.NewToday)
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE created_at >= $1 AND created_at < $2`, yesterdayStart, todayStart).Scan(&stats.NewYesterday)
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE created_at >= $1`, now.Add(-7*24*time.Hour)).Scan(&stats.New7d)
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE created_at >= $1`, now.Add(-30*24*time.Hour)).Scan(&stats.New30d)

	_ = s.pool.QueryRow(ctx, `SELECT COUNT(DISTINCT user_id) FROM downloads WHERE created_at >= $1`, now.Add(-24*time.Hour)).Scan(&stats.DAU)
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(DISTINCT user_id) FROM downloads WHERE created_at >= $1`, now.Add(-7*24*time.Hour)).Scan(&stats.WAU)
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(DISTINCT user_id) FROM downloads WHERE created_at >= $1`, now.Add(-30*24*time.Hour)).Scan(&stats.MAU)
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(DISTINCT user_id) FROM downloads`).Scan(&stats.UsersWithDownloads)
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM (
			SELECT user_id FROM downloads WHERE status = 'completed'::downloadstatus
			GROUP BY user_id HAVING COUNT(*) >= 2
		) t
	`).Scan(&stats.ReturningUsers)

	rows, err := s.pool.Query(ctx, `SELECT language::text, COUNT(*) FROM users GROUP BY language`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var lang string
			var count int
			if rows.Scan(&lang, &count) == nil {
				switch lang {
				case "en":
					stats.LanguageEN = count
				case "ru":
					stats.LanguageRU = count
				}
			}
		}
	}

	rows2, err := s.pool.Query(ctx, `
		SELECT platform::text, COUNT(DISTINCT user_id)
		FROM downloads WHERE created_at >= $1
		GROUP BY platform ORDER BY COUNT(DISTINCT user_id) DESC LIMIT 3
	`, now.Add(-7*24*time.Hour))
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var pc PlatformCount
			if rows2.Scan(&pc.Platform, &pc.Count) == nil {
				stats.TopPlatforms7d = append(stats.TopPlatforms7d, pc)
			}
		}
	}

	return stats, nil
}
