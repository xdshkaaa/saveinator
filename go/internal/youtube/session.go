package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	sessionTTL = 15 * time.Minute
	metaTTL    = time.Hour
)

// PendingSession is the state behind one format card: which video it describes,
// what it offered, and whether the user is currently typing a trim range.
type PendingSession struct {
	UserID      int64    `json:"user_id"`
	URL         string   `json:"url"`
	ChatID      int64    `json:"chat_id"`
	MessageID   int      `json:"message_id"`
	Lang        string   `json:"lang"`
	Title       string   `json:"title,omitempty"`
	Author      string   `json:"author,omitempty"`
	DurationSec int      `json:"duration_sec,omitempty"`
	Options     []Option `json:"options,omitempty"`

	AwaitingTrim bool    `json:"awaiting_trim,omitempty"`
	TrimStart    float64 `json:"trim_start,omitempty"`
	TrimEnd      float64 `json:"trim_end,omitempty"`
}

// Meta rebuilds the metadata needed to re-render the card without a new probe.
func (s PendingSession) Meta() *Meta {
	return &Meta{Title: s.Title, Uploader: s.Author, Duration: float64(s.DurationSec)}
}

// TrimLabel renders the chosen fragment, or "" when the whole video is wanted.
func (s PendingSession) TrimLabel() string {
	if s.TrimEnd <= s.TrimStart {
		return ""
	}
	return FormatRange(s.TrimStart, s.TrimEnd)
}

// OptionFor returns the exact format selector advertised for a height.
func (s PendingSession) OptionFor(height int) (Option, bool) {
	for _, opt := range s.Options {
		if opt.Height == height {
			return opt, true
		}
	}
	return Option{}, false
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

// SetAwaitingTrim flips the "user is typing a range" flag on the live session.
func (s *SessionStore) SetAwaitingTrim(ctx context.Context, userID int64, awaiting bool) (*PendingSession, error) {
	return s.mutate(ctx, userID, func(session *PendingSession) {
		session.AwaitingTrim = awaiting
	})
}

// SetTrim stores a parsed fragment and clears the awaiting flag.
func (s *SessionStore) SetTrim(ctx context.Context, userID int64, start, end float64) (*PendingSession, error) {
	return s.mutate(ctx, userID, func(session *PendingSession) {
		session.AwaitingTrim = false
		session.TrimStart = start
		session.TrimEnd = end
	})
}

func (s *SessionStore) mutate(ctx context.Context, userID int64, apply func(*PendingSession)) (*PendingSession, error) {
	session, err := s.Get(ctx, userID)
	if err != nil || session == nil {
		return nil, err
	}
	apply(session)
	if err := s.Save(ctx, *session); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *SessionStore) Clear(ctx context.Context, userID int64) error {
	return s.rdb.Del(ctx, key(userID)).Err()
}

// GetMeta returns cached probe output for a video, or nil on a miss.
func (s *SessionStore) GetMeta(ctx context.Context, videoID string) *Meta {
	if videoID == "" {
		return nil
	}
	raw, err := s.rdb.Get(ctx, metaKey(videoID)).Result()
	if err != nil {
		return nil
	}
	meta, err := ParseMeta([]byte(raw))
	if err != nil {
		return nil
	}
	return meta
}

// SaveMeta caches probe output so re-sent links skip the yt-dlp round trip.
func (s *SessionStore) SaveMeta(ctx context.Context, meta *Meta) {
	if meta == nil || meta.ID == "" {
		return
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return
	}
	_ = s.rdb.Set(ctx, metaKey(meta.ID), raw, metaTTL).Err()
}

func key(userID int64) string {
	return fmt.Sprintf("yt_pending:%d", userID)
}

func metaKey(videoID string) string {
	return "yt_meta:" + videoID
}
