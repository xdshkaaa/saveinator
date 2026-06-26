package queue

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

const (
	TypeDownload = "download:send"
	TypeTikTok   = "download:tiktok"
)

type DownloadPayload struct {
	URL        string `json:"url"`
	Platform   string `json:"platform"`
	ChatID     int64  `json:"chat_id"`
	UserID     int64  `json:"user_id"`
	MessageID  int    `json:"message_id"`
	Lang       string `json:"lang"`
	LockToken  string `json:"lock_token"`
	LockScene  string `json:"lock_scene"`
	XStatusID  string `json:"x_status_id,omitempty"`
	FormatID   string `json:"format_id,omitempty"`
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
	task := asynq.NewTask(TypeDownload, body)
	_, err = c.client.Enqueue(task, asynq.MaxRetry(0), asynq.Timeout(30*time.Minute))
	return err
}

func (c *Client) EnqueueTikTok(p DownloadPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	task := asynq.NewTask(TypeTikTok, body)
	_, err = c.client.Enqueue(task, asynq.MaxRetry(0), asynq.Timeout(30*time.Minute))
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
