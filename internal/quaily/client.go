package quaily

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a minimal HTTP client for Quaily API.
type Client struct {
    baseURL string
    apiKey  string
    http    *http.Client
    // Endpoints (optional overrides)
    createPath  string
    publishPath string // Template: "/posts/%s/publish"
    deliverPath string // Template: "/lists/%s/posts/%s/deliver"
}

// New creates a new Quaily client.
// baseURL should be like "https://api.quaily.com/v1" (no trailing slash).
func New(baseURL, apiKey string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
        return &Client{
            baseURL:     strings.TrimRight(baseURL, "/"),
            apiKey:      apiKey,
            http:        &http.Client{Timeout: timeout},
            createPath:  "/lists/%s/posts",
            publishPath: "/lists/%s/posts/%s/publish",
            deliverPath: "/lists/%s/posts/%s/deliver",
        }
}

// WithPaths optionally overrides endpoints.
func (c *Client) WithPaths(createPath, publishPath string) *Client {
    c2 := *c
    if strings.TrimSpace(createPath) != "" {
        c2.createPath = createPath
    }
    if strings.TrimSpace(publishPath) != "" {
        c2.publishPath = publishPath
    }
    return &c2
}

// WithDeliverPath optionally overrides the deliver endpoint path.
func (c *Client) WithDeliverPath(deliverPath string) *Client {
    c2 := *c
    if strings.TrimSpace(deliverPath) != "" {
        c2.deliverPath = deliverPath
    }
    return &c2
}

// CreatePost sends a Create Post request to Quaily.
// params should contain the post fields; caller should include channel_slug and content.
// Returns the created post ID as string.
func (c *Client) CreatePost(ctx context.Context, channelSlug string, params map[string]any) (string, error) {
	if c == nil {
		return "", errors.New("nil quaily client")
	}
	body, err := json.Marshal(params)
	if err != nil {
		return "", err
	}

	url := c.baseURL + fmt.Sprintf(c.createPath, channelSlug)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create post failed: status=%d body=%s", resp.StatusCode, string(b))
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	// Try common patterns for id
	if id, ok := out["id"].(string); ok && id != "" {
		return id, nil
	}
	if idf, ok := out["id"].(float64); ok {
		return fmt.Sprintf("%v", idf), nil
	}
	if data, ok := out["data"].(map[string]any); ok {
		if id, ok := data["id"].(string); ok && id != "" {
			return id, nil
		}
		if idf, ok := data["id"].(float64); ok {
			return fmt.Sprintf("%v", idf), nil
		}
	}
	return "", errors.New("create post: missing id in response")
}

// PublishPost triggers publishing for a post by ID.
func (c *Client) PublishPost(ctx context.Context, channelSlug, id string) error {
	if c == nil {
		return errors.New("nil quaily client")
	}
	if strings.TrimSpace(id) == "" {
		return errors.New("empty post id")
	}
	url := c.baseURL + fmt.Sprintf(c.publishPath, channelSlug, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("publish post failed: status=%d body=%s", resp.StatusCode, string(b))
	}
	return nil
}

// DeliverPost triggers delivery (send) for a post by slug.
func (c *Client) DeliverPost(ctx context.Context, channelSlug, postSlug string) error {
    if c == nil {
        return errors.New("nil quaily client")
    }
    if strings.TrimSpace(postSlug) == "" {
        return errors.New("empty post slug")
    }
    url := c.baseURL + fmt.Sprintf(c.deliverPath, channelSlug, postSlug)
    req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, http.NoBody)
    if err != nil {
        return err
    }
    req.Header.Set("Authorization", "Bearer "+c.apiKey)
    req.Header.Set("Content-Type", "application/json")
    resp, err := c.http.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        b, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("deliver post failed: status=%d body=%s", resp.StatusCode, string(b))
    }
    return nil
}
