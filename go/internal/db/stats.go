package db

import (
	"context"
	"time"
)

const activityDownloadFilter = `status = 'COMPLETED'::downloadstatus`

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
	DownloadsToday     int
	Downloads7d        int
	Downloads30d       int
	Completed30d       int
	Failed30d          int
	LanguageEN         int
	LanguageRU         int
	LanguageKK         int
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
	d7 := now.Add(-7 * 24 * time.Hour)
	d30 := now.Add(-30 * 24 * time.Hour)
	d24 := now.Add(-24 * time.Hour)

	err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE created_at >= $1) AS new_today,
			COUNT(*) FILTER (WHERE created_at >= $2 AND created_at < $1) AS new_yesterday,
			COUNT(*) FILTER (WHERE created_at >= $3) AS new_7d,
			COUNT(*) FILTER (WHERE created_at >= $4) AS new_30d,
			COUNT(*) FILTER (WHERE language = 'EN'::language) AS lang_en,
			COUNT(*) FILTER (WHERE language = 'RU'::language) AS lang_ru,
			COUNT(*) FILTER (WHERE language = 'KK'::language) AS lang_kk
		FROM users
	`, todayStart, yesterdayStart, d7, d30).Scan(
		&stats.TotalUsers,
		&stats.NewToday,
		&stats.NewYesterday,
		&stats.New7d,
		&stats.New30d,
		&stats.LanguageEN,
		&stats.LanguageRU,
		&stats.LanguageKK,
	)
	if err != nil {
		return stats, err
	}

	err = s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE `+activityDownloadFilter+` AND created_at >= $1) AS downloads_today,
			COUNT(*) FILTER (WHERE `+activityDownloadFilter+` AND created_at >= $2) AS downloads_7d,
			COUNT(*) FILTER (WHERE `+activityDownloadFilter+` AND created_at >= $3) AS downloads_30d,
			COUNT(*) FILTER (WHERE `+activityDownloadFilter+` AND created_at >= $3) AS completed_30d,
			COUNT(*) FILTER (WHERE status = 'FAILED'::downloadstatus
				AND created_at >= $3) AS failed_30d
		FROM downloads
	`, todayStart, d7, d30).Scan(
		&stats.DownloadsToday,
		&stats.Downloads7d,
		&stats.Downloads30d,
		&stats.Completed30d,
		&stats.Failed30d,
	)
	if err != nil {
		return stats, err
	}

	err = s.pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(DISTINCT user_id) FROM downloads
				WHERE `+activityDownloadFilter+` AND created_at >= $1) AS dau,
			(SELECT COUNT(DISTINCT user_id) FROM downloads
				WHERE `+activityDownloadFilter+` AND created_at >= $2) AS wau,
			(SELECT COUNT(DISTINCT user_id) FROM downloads
				WHERE `+activityDownloadFilter+` AND created_at >= $3) AS mau,
			(SELECT COUNT(DISTINCT user_id) FROM downloads
				WHERE `+activityDownloadFilter+`) AS users_with_downloads,
			(SELECT COUNT(*) FROM (
				SELECT user_id FROM downloads
				WHERE `+activityDownloadFilter+`
				GROUP BY user_id HAVING COUNT(*) >= 2
			) t) AS returning_users
	`, d24, d7, d30).Scan(
		&stats.DAU,
		&stats.WAU,
		&stats.MAU,
		&stats.UsersWithDownloads,
		&stats.ReturningUsers,
	)
	if err != nil {
		return stats, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT platform::text, COUNT(DISTINCT user_id)
		FROM downloads
		WHERE `+activityDownloadFilter+` AND created_at >= $1
		GROUP BY platform ORDER BY COUNT(DISTINCT user_id) DESC LIMIT 3
	`, d7)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var pc PlatformCount
			if rows.Scan(&pc.Platform, &pc.Count) == nil {
				stats.TopPlatforms7d = append(stats.TopPlatforms7d, pc)
			}
		}
	}

	return stats, nil
}
