package telegraph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"saveinator/internal/redisx"
)

// apiBase is a var so tests can point the client at an httptest server.
// api.telegra.ph is the official endpoint; api.telegraph.me is dead from
// at least some hosts (connection reset), which silently killed publishing.
var apiBase = "https://api.telegra.ph"

// Node is one Telegraph content node. Telegraph nodes form a tree where
// children are either plain strings or nested nodes, hence the []any.
type Node struct {
	Tag      string            `json:"tag,omitempty"`
	Attrs    map[string]string `json:"attrs,omitempty"`
	Children []any             `json:"children,omitempty"`
}

type Client struct {
	client *http.Client
}

func NewClient() *Client {
	return &Client{client: &http.Client{Timeout: 20 * time.Second}}
}

// CreateAccount registers a throwaway Telegraph account and returns its
// access token. shortName is the account handle shown in page credits.
func (c *Client) CreateAccount(ctx context.Context, shortName, authorName string) (string, error) {
	params := url.Values{}
	params.Set("short_name", shortName)
	if authorName != "" {
		params.Set("author_name", authorName)
	}

	var result struct {
		Ok     bool `json:"ok"`
		Error  string `json:"error"`
		Result struct {
			AccessToken string `json:"access_token"`
		} `json:"result"`
	}
	if err := c.post(ctx, "createAccount", params, &result); err != nil {
		return "", err
	}
	if !result.Ok || result.Result.AccessToken == "" {
		return "", fmt.Errorf("telegraph: createAccount failed: %s", result.Error)
	}
	return result.Result.AccessToken, nil
}

// CreatePage publishes a page and returns its public URL.
func (c *Client) CreatePage(ctx context.Context, token, title string, content []Node, authorName, authorURL string) (string, error) {
	if token == "" {
		return "", fmt.Errorf("telegraph: no access token")
	}
	contentJSON, err := json.Marshal(content)
	if err != nil {
		return "", err
	}

	params := url.Values{}
	params.Set("access_token", token)
	params.Set("title", title)
	params.Set("author_name", authorName)
	if authorURL != "" {
		params.Set("author_url", authorURL)
	}
	params.Set("content", string(contentJSON))
	params.Set("return_content", "false")

	var result struct {
		Ok     bool `json:"ok"`
		Error  string `json:"error"`
		Result struct {
			URL  string `json:"url"`
			Path string `json:"path"`
		} `json:"result"`
	}
	if err := c.post(ctx, "createPage", params, &result); err != nil {
		return "", err
	}
	if !result.Ok || result.Result.URL == "" {
		return "", fmt.Errorf("telegraph: createPage failed: %s", result.Error)
	}
	return result.Result.URL, nil
}

func (c *Client) post(ctx context.Context, method string, params url.Values, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/"+method,
		strings.NewReader(params.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, into); err != nil {
		return fmt.Errorf("telegraph: %s: decode response: %w", method, err)
	}
	return nil
}

const accountTokenKey = "telegraph:account_token"

// ResolveToken returns the configured Telegraph token, lazily creating an
// account and persisting the token in Redis (no expiry) when TELEGRAPH_ACCESS_TOKEN
// is not set, so restarts reuse the same account.
func ResolveToken(ctx context.Context, cfgToken, authorName string, redis *redisx.Client) (string, error) {
	if cfgToken != "" {
		return cfgToken, nil
	}
	if rdb := redis.Raw(); rdb != nil {
		if tok, err := rdb.Get(ctx, accountTokenKey).Result(); err == nil && tok != "" {
			return tok, nil
		}
	}
	token, err := NewClient().CreateAccount(ctx, "saveinator", authorName)
	if err != nil {
		return "", err
	}
	if rdb := redis.Raw(); rdb != nil {
		if err := rdb.Set(ctx, accountTokenKey, token, 0).Err(); err != nil {
			return "", fmt.Errorf("telegraph: persist token: %w", err)
		}
	}
	return token, nil
}
