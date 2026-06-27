package db

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (s *Store) IsLinkBanned(ctx context.Context, urlHash string) (bool, error) {
	if len(urlHash) > 64 {
		urlHash = urlHash[:64]
	}
	var id int
	err := s.pool.QueryRow(ctx, `SELECT id FROM banned_links WHERE url_hash = $1 LIMIT 1`, urlHash).Scan(&id)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
