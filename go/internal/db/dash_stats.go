package db

import (
	"context"
	"strconv"
	"time"
)

const dashCompleted = `status = 'COMPLETED'::downloadstatus`

type BotStats struct {
	Slug          string `json:"slug"`
	Users         int    `json:"users"`
	Downloads     int    `json:"downloads"`
	Downloads30d  int    `json:"downloads_30d"`
	Failed30d     int    `json:"failed_30d"`
}

type PlatformStat struct {
	Platform      string  `json:"platform"`
	Downloads     int     `json:"downloads"`
	Downloads30d  int     `json:"downloads_30d"`
	Users         int     `json:"users"`
	Completed30d  int     `json:"completed_30d"`
	Failed30d     int     `json:"failed_30d"`
}

type LangStat struct {
	Language string `json:"language"`
	Users    int    `json:"users"`
}

type Overview struct {
	TotalUsers     int            `json:"total_users"`
	NewToday       int            `json:"new_today"`
	NewYesterday   int            `json:"new_yesterday"`
	New7d          int            `json:"new_7d"`
	New30d         int            `json:"new_30d"`
	DownloadsToday int            `json:"downloads_today"`
	Downloads7d    int            `json:"downloads_7d"`
	Downloads30d   int            `json:"downloads_30d"`
	Completed30d   int            `json:"completed_30d"`
	Failed30d      int            `json:"failed_30d"`
	DAU            int            `json:"dau"`
	WAU            int            `json:"wau"`
	MAU            int            `json:"mau"`
	UsersWith      int            `json:"users_with_downloads"`
	ReturningUsers int            `json:"returning_users"`
	Languages      []LangStat     `json:"languages"`
	Platforms30d   []PlatformStat `json:"platforms_30d"`
	Bots           []BotStats     `json:"bots"`
}

// DashOverview aggregates user and download metrics across all bots.
func (s *Store) DashOverview(ctx context.Context) (Overview, error) {
	var o Overview
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
			COUNT(*) FILTER (WHERE created_at >= $4) AS new_30d
		FROM users
	`, todayStart, yesterdayStart, d7, d30).Scan(
		&o.TotalUsers, &o.NewToday, &o.NewYesterday, &o.New7d, &o.New30d,
	)
	if err != nil {
		return o, err
	}

	err = s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE `+dashCompleted+` AND created_at >= $1) AS downloads_today,
			COUNT(*) FILTER (WHERE `+dashCompleted+` AND created_at >= $2) AS downloads_7d,
			COUNT(*) FILTER (WHERE `+dashCompleted+` AND created_at >= $3) AS downloads_30d,
			COUNT(*) FILTER (WHERE `+dashCompleted+` AND created_at >= $3) AS completed_30d,
			COUNT(*) FILTER (WHERE status = 'FAILED'::downloadstatus AND created_at >= $3) AS failed_30d
		FROM downloads
	`, todayStart, d7, d30).Scan(
		&o.DownloadsToday, &o.Downloads7d, &o.Downloads30d, &o.Completed30d, &o.Failed30d,
	)
	if err != nil {
		return o, err
	}

	err = s.pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(DISTINCT user_id) FROM downloads
				WHERE `+dashCompleted+` AND created_at >= $1) AS dau,
			(SELECT COUNT(DISTINCT user_id) FROM downloads
				WHERE `+dashCompleted+` AND created_at >= $2) AS wau,
			(SELECT COUNT(DISTINCT user_id) FROM downloads
				WHERE `+dashCompleted+` AND created_at >= $3) AS mau,
			(SELECT COUNT(DISTINCT user_id) FROM downloads
				WHERE `+dashCompleted+`) AS users_with,
			(SELECT COUNT(*) FROM (
				SELECT user_id FROM downloads
				WHERE `+dashCompleted+`
				GROUP BY user_id HAVING COUNT(*) >= 2
			) t) AS returning
	`, d24, d7, d30).Scan(
		&o.DAU, &o.WAU, &o.MAU, &o.UsersWith, &o.ReturningUsers,
	)
	if err != nil {
		return o, err
	}

	langRows, err := s.pool.Query(ctx, `
		SELECT language::text, COUNT(*)
		FROM users
		GROUP BY language
		ORDER BY COUNT(*) DESC
	`)
	if err != nil {
		return o, err
	}
	defer langRows.Close()
	for langRows.Next() {
		var l LangStat
		if langRows.Scan(&l.Language, &l.Users) == nil {
			o.Languages = append(o.Languages, l)
		}
	}

	platRows, err := s.pool.Query(ctx, `
		SELECT platform::text,
			COUNT(*) AS downloads,
			COUNT(*) FILTER (WHERE created_at >= $1) AS downloads_30d,
			COUNT(DISTINCT user_id) AS users,
			COUNT(*) FILTER (WHERE `+dashCompleted+` AND created_at >= $1) AS completed_30d,
			COUNT(*) FILTER (WHERE status = 'FAILED'::downloadstatus AND created_at >= $1) AS failed_30d
		FROM downloads
		GROUP BY platform
		ORDER BY downloads_30d DESC
	`, d30)
	if err != nil {
		return o, err
	}
	defer platRows.Close()
	for platRows.Next() {
		var p PlatformStat
		if platRows.Scan(&p.Platform, &p.Downloads, &p.Downloads30d, &p.Users, &p.Completed30d, &p.Failed30d) == nil {
			o.Platforms30d = append(o.Platforms30d, p)
		}
	}

	botUsers, err := s.pool.Query(ctx, `
		SELECT COALESCE(bot_id, 'unknown'), COUNT(*)
		FROM users
		GROUP BY bot_id
		ORDER BY COUNT(*) DESC
	`)
	if err != nil {
		return o, err
	}
	defer botUsers.Close()
	userCounts := map[string]int{}
	for botUsers.Next() {
		var slug string
		var n int
		if botUsers.Scan(&slug, &n) == nil {
			userCounts[slug] = n
		}
	}

	botRows, err := s.pool.Query(ctx, `
		SELECT COALESCE(bot_id, 'unknown'),
			COUNT(*) AS downloads,
			COUNT(*) FILTER (WHERE created_at >= $1) AS downloads_30d,
			COUNT(*) FILTER (WHERE status = 'FAILED'::downloadstatus AND created_at >= $1) AS failed_30d
		FROM downloads
		GROUP BY bot_id
		ORDER BY downloads DESC
	`, d30)
	if err != nil {
		return o, err
	}
	defer botRows.Close()
	for botRows.Next() {
		var b BotStats
		if botRows.Scan(&b.Slug, &b.Downloads, &b.Downloads30d, &b.Failed30d) == nil {
			b.Users = userCounts[b.Slug]
			o.Bots = append(o.Bots, b)
		}
	}

	return o, nil
}

type DayPoint struct {
	Day        string `json:"day"`
	Total      int    `json:"total"`
	Completed  int    `json:"completed"`
	Failed     int    `json:"failed"`
	Unique     int    `json:"unique_users"`
}

// DownloadTimeline returns per-day totals for the last N days (UTC).
func (s *Store) DownloadTimeline(ctx context.Context, days int) ([]DayPoint, error) {
	if days < 1 || days > 365 {
		days = 14
	}
	rows, err := s.pool.Query(ctx, `
		SELECT to_char(date_trunc('day', created_at), 'YYYY-MM-DD') AS day,
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE `+dashCompleted+`) AS completed,
			COUNT(*) FILTER (WHERE status = 'FAILED'::downloadstatus) AS failed,
			COUNT(DISTINCT user_id) AS unique_users
		FROM downloads
		WHERE created_at >= date_trunc('day', now()) - make_interval(days => $1)
		GROUP BY day
		ORDER BY day
	`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := make([]DayPoint, 0, days)
	for rows.Next() {
		var p DayPoint
		if rows.Scan(&p.Day, &p.Total, &p.Completed, &p.Failed, &p.Unique) == nil {
			points = append(points, p)
		}
	}
	return points, rows.Err()
}

type PlatformRow struct {
	Platform     string `json:"platform"`
	Downloads    int    `json:"downloads"`
	Completed    int    `json:"completed"`
	Failed       int    `json:"failed"`
	Users        int    `json:"users"`
}

// PlatformBreakdown returns per-platform totals for the last N days.
func (s *Store) PlatformBreakdown(ctx context.Context, days int) ([]PlatformRow, error) {
	if days < 1 || days > 365 {
		days = 30
	}
	rows, err := s.pool.Query(ctx, `
		SELECT platform::text,
			COUNT(*) AS downloads,
			COUNT(*) FILTER (WHERE `+dashCompleted+`) AS completed,
			COUNT(*) FILTER (WHERE status = 'FAILED'::downloadstatus) AS failed,
			COUNT(DISTINCT user_id) AS users
		FROM downloads
		WHERE created_at >= date_trunc('day', now()) - make_interval(days => $1)
		GROUP BY platform
		ORDER BY downloads DESC
	`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PlatformRow
	for rows.Next() {
		var r PlatformRow
		if rows.Scan(&r.Platform, &r.Downloads, &r.Completed, &r.Failed, &r.Users) == nil {
			out = append(out, r)
		}
	}
	return out, rows.Err()
}

type UserRow struct {
	ID           int64   `json:"id"`
	Username     *string `json:"username"`
	FirstName    *string `json:"first_name"`
	Language     *string `json:"language"`
	BotID        *string `json:"bot_id"`
	CreatedAt    time.Time `json:"created_at"`
	Downloads    int       `json:"downloads"`
	Completed    int       `json:"completed"`
	Failed       int       `json:"failed"`
	LastActivity *time.Time `json:"last_activity"`
}

// UserTable lists users with per-user download counters, searchable and sortable.
func (s *Store) UserTable(ctx context.Context, sort, q string, limit int) ([]UserRow, error) {
	if limit < 1 || limit > 1000 {
		limit = 200
	}
	order := "u.created_at DESC"
	switch sort {
	case "downloads":
		order = "COUNT(d.id) DESC, u.created_at DESC"
	case "completed":
		order = "COUNT(d.id) FILTER (WHERE " + dashCompleted + ") DESC, u.created_at DESC"
	}

	query := `
		SELECT u.id, u.username, u.first_name, u.language::text, u.bot_id,
			u.created_at,
			COUNT(d.id) AS downloads,
			COUNT(d.id) FILTER (WHERE ` + dashCompleted + `) AS completed,
			COUNT(d.id) FILTER (WHERE d.status = 'FAILED'::downloadstatus) AS failed,
			MAX(d.created_at) AS last_activity
		FROM users u
		LEFT JOIN downloads d ON d.user_id = u.id
	`
	args := []any{}
	if q != "" {
		query += `WHERE (u.username ILIKE '%' || $1 || '%' OR u.first_name ILIKE '%' || $1 || '%')
`
		args = append(args, q)
	}
	query += `GROUP BY u.id
		ORDER BY ` + order + `
		LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UserRow
	for rows.Next() {
		var r UserRow
		if rows.Scan(&r.ID, &r.Username, &r.FirstName, &r.Language, &r.BotID,
			&r.CreatedAt, &r.Downloads, &r.Completed, &r.Failed, &r.LastActivity) == nil {
			out = append(out, r)
		}
	}
	return out, rows.Err()
}

