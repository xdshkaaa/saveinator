package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const sessionTTL = 15 * time.Minute

type PendingSession struct {
	UserID    int64  `json:"user_id"`
	URL       string `json:"url"`
	ChatID    int64  `json:"chat_id"`
	MessageID int    `json:"message_id"`
	Lang      string `json:"lang"`
	Quality   *int   `json:"quality,omitempty"`
}

type SessionStore struct {
	rdb *redis.Client
}

func NewSessionStore(rdb *redis.Client) *SessionStore {
	return &SessionStore{rdb: rdb}
}

func (s *SessionStore) Save(ctx context.Context, session PendingSession) error {
	raw, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, key(session.UserID), raw, sessionTTL).Err()
}

func (s *SessionStore) Get(ctx context.Context, userID int64) (*PendingSession, error) {
	raw, err := s.rdb.Get(ctx, key(userID)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var session PendingSession
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		return nil, nil
	}
	return &session, nil
}

func (s *SessionStore) UpdateQuality(ctx context.Context, userID int64, quality int) (*PendingSession, error) {
	session, err := s.Get(ctx, userID)
	if err != nil || session == nil {
		return nil, err
	}
	session.Quality = &quality
	if err := s.Save(ctx, *session); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *SessionStore) Clear(ctx context.Context, userID int64) error {
	return s.rdb.Del(ctx, key(userID)).Err()
}

func key(userID int64) string {
	return fmt.Sprintf("yt_pending:%d", userID)
}
