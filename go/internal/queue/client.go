package queue

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

const (
	TypeDownload    = "download:send"
	TypeTikTok      = "download:tiktok"
	TypePinterest   = "download:pinterest"
	TypePinterestKZ = "download:pinterest_kz"
	TypeSpotify     = "download:spotify"
	TypeSoundCloud  = "download:soundcloud"
	TypeYandexMusic = "download:yandexmusic"
	TypeBroadcast       = "broadcast:execute"
	TypeTikTokCarousel  = "download:tiktok_carousel"
	TypeInstagram       = "download:instagram"

	// QueueDownload holds network-bound jobs (fetch + upload, no ffmpeg
	// transcode): safe to run with higher concurrency on a single CPU.
	QueueDownload = "download"
	// QueueTranscode holds the one CPU-bound path (YouTube quality/ratio
	// re-encode via ffmpeg) and stays single-concurrency to avoid CPU
	// contention with itself or with QueueDownload uploads.
	QueueTranscode = "transcode"
)

// QueueFor routes a download payload. Only jobs that will hit the ffmpeg
// transcode path (runYouTubeDownload re-encoding to an explicit aspect ratio)
// belong on the single-concurrency transcode queue; plain downloads and
// audio-only jobs never re-encode video and stay on the parallel queue.
func QueueFor(p DownloadPayload) string {
	if p.Platform == "youtube" && !p.AudioOnly && p.Quality > 0 && p.AspectRatio != "" {
		return QueueTranscode
	}
	return QueueDownload
}

type DownloadPayload struct {
	URL        string `json:"url"`
	Platform   string `json:"platform"`
	ChatID     int64  `json:"chat_id"`
	UserID     int64  `json:"user_id"`
	MessageID  int    `json:"message_id"`
	Lang       string `json:"lang"`
	LockToken  string `json:"lock_token"`
	LockScene  string `json:"lock_scene"`
	XStatusID   string `json:"x_status_id,omitempty"`
	FormatID    string `json:"format_id,omitempty"`
	Quality     int    `json:"quality,omitempty"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	SessionToken string `json:"session_token,omitempty"`

	// Title and Author carry probed metadata so the worker can label an audio
	// upload without probing the video a second time.
	Title  string `json:"title,omitempty"`
	Author string `json:"author,omitempty"`
	// AudioOnly requests the soundtrack instead of the video.
	AudioOnly bool `json:"audio_only,omitempty"`
	// TrimStart/TrimEnd bound the fragment to fetch, in seconds. Zero means
	// "whole video".
	TrimStart float64 `json:"trim_start,omitempty"`
	TrimEnd   float64 `json:"trim_end,omitempty"`
}

// IsTrimmed reports whether the payload asks for a fragment rather than the
// whole video.
func (p DownloadPayload) IsTrimmed() bool {
	return p.TrimEnd > p.TrimStart
}

type MusicPayload struct {
	Platform   string `json:"platform"`
	ChatID     int64  `json:"chat_id"`
	UserID     int64  `json:"user_id"`
	MessageID  int    `json:"message_id"`
	Lang       string `json:"lang"`
	LockToken  string `json:"lock_token"`
	LockScene  string `json:"lock_scene"`
	LinkType   string `json:"link_type,omitempty"`
	ResourceID string `json:"resource_id,omitempty"`
	SourceURL  string `json:"source_url,omitempty"`
	ReleaseJSON string `json:"release_json"`
}

type BroadcastPayload struct {
	BroadcastID int     `json:"broadcast_id"`
	Audience    string  `json:"audience"`
	UserIDs     []int64 `json:"user_ids"`
}

type Client struct {
	client *asynq.Client
}

func NewClient(redisURL string) (*Client, error) {
	opt, err := redisClientOpt(redisURL)
	if err != nil {
		return nil, err
	}
	return &Client{client: asynq.NewClient(opt)}, nil
}

func (c *Client) Close() error {
	return c.client.Close()
}

func (c *Client) EnqueueDownload(p DownloadPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	queueName := QueueFor(p)
	task := asynq.NewTask(TypeDownload, body)
	_, err = c.client.Enqueue(task, asynq.MaxRetry(1), asynq.Timeout(30*time.Minute), asynq.Queue(queueName))
	return err
}

// EnqueueDownloadTo enqueues a download task with an explicit task type and
// queue name; used by botkit bots whose queue comes from BotConfig.
func (c *Client) EnqueueDownloadTo(taskType, queueName string, p DownloadPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	task := asynq.NewTask(taskType, body)
	_, err = c.client.Enqueue(task, asynq.MaxRetry(1), asynq.Timeout(30*time.Minute), asynq.Queue(queueName))
	return err
}

// EnqueueMusicTo is the MusicPayload counterpart of EnqueueDownloadTo.
func (c *Client) EnqueueMusicTo(taskType, queueName string, p MusicPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	task := asynq.NewTask(taskType, body)
	_, err = c.client.Enqueue(task, asynq.MaxRetry(0), asynq.Timeout(2*time.Hour), asynq.Queue(queueName))
	return err
}

func (c *Client) EnqueuePinterestDefault(p DownloadPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	task := asynq.NewTask(TypePinterest, body)
	_, err = c.client.Enqueue(task, asynq.MaxRetry(1), asynq.Timeout(30*time.Minute), asynq.Queue(QueueDownload))
	return err
}

func (c *Client) EnqueueTikTok(p DownloadPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	task := asynq.NewTask(TypeTikTok, body)
	_, err = c.client.Enqueue(task, asynq.MaxRetry(1), asynq.Timeout(30*time.Minute), asynq.Queue(QueueDownload))
	return err
}

func (c *Client) EnqueueSpotify(p MusicPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	task := asynq.NewTask(TypeSpotify, body)
	_, err = c.client.Enqueue(task, asynq.MaxRetry(0), asynq.Timeout(2*time.Hour), asynq.Queue(QueueDownload))
	return err
}

func (c *Client) EnqueueSoundCloud(p MusicPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	task := asynq.NewTask(TypeSoundCloud, body)
	_, err = c.client.Enqueue(task, asynq.MaxRetry(0), asynq.Timeout(2*time.Hour), asynq.Queue(QueueDownload))
	return err
}

func (c *Client) EnqueueYandexMusic(p MusicPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	task := asynq.NewTask(TypeYandexMusic, body)
	_, err = c.client.Enqueue(task, asynq.MaxRetry(0), asynq.Timeout(2*time.Hour), asynq.Queue(QueueDownload))
	return err
}

func (c *Client) EnqueueTikTokCarousel(p DownloadPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	task := asynq.NewTask(TypeTikTokCarousel, body)
	_, err = c.client.Enqueue(task, asynq.MaxRetry(1), asynq.Timeout(30*time.Minute), asynq.Queue(QueueDownload))
	return err
}

func (c *Client) EnqueueInstagram(p DownloadPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	task := asynq.NewTask(TypeInstagram, body)
	_, err = c.client.Enqueue(task, asynq.MaxRetry(1), asynq.Timeout(30*time.Minute), asynq.Queue(QueueDownload))
	return err
}

func (c *Client) EnqueueBroadcast(p BroadcastPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	task := asynq.NewTask(TypeBroadcast, body)
	_, err = c.client.Enqueue(task, asynq.MaxRetry(1), asynq.Timeout(24*time.Hour))
	return err
}

func RedisOpt(redisURL string) (asynq.RedisClientOpt, error) {
	return redisClientOpt(redisURL)
}

func redisClientOpt(redisURL string) (asynq.RedisClientOpt, error) {
	rconn, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return asynq.RedisClientOpt{}, fmt.Errorf("parse redis: %w", err)
	}
	opt, ok := rconn.(asynq.RedisClientOpt)
	if !ok {
		return asynq.RedisClientOpt{}, fmt.Errorf("unsupported redis URI scheme (cluster/sentinel)")
	}
	return opt, nil
}
