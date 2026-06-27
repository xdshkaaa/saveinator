package tiktok

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const sessionPrefix = "ttk_carousel"
const sessionTTL = 15 * time.Minute

type CarouselSession struct {
	UserID int64  `json:"user_id"`
	URL    string `json:"url"`
	ChatID int64  `json:"chat_id"`
	Lang   string `json:"lang"`
	Title  string `json:"title"`
	Author string `json:"author"`
	Token  string `json:"token"`
}

type SessionStore struct {
	rdb *redis.Client
}

func NewSessionStore(rdb *redis.Client) *SessionStore {
	return &SessionStore{rdb: rdb}
}

func (s *SessionStore) Save(ctx context.Context, session *CarouselSession) error {
	body, err := json.Marshal(session)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("%s:%s", sessionPrefix, session.Token)
	return s.rdb.Set(ctx, key, body, sessionTTL).Err()
}

func (s *SessionStore) Get(ctx context.Context, token string) (*CarouselSession, error) {
	raw, err := s.rdb.Get(ctx, fmt.Sprintf("%s:%s", sessionPrefix, token)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var session CarouselSession
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *SessionStore) Delete(ctx context.Context, token string) error {
	return s.rdb.Del(ctx, fmt.Sprintf("%s:%s", sessionPrefix, token)).Err()
}
