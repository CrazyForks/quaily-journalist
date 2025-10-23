package v2ex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"quaily-journalist/internal/model"
)

type Client struct {
	baseURL string
	client  *http.Client
	token   string
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
		token:   token,
	}
}

// Topic represents a subset of V2EX topic fields used by this service.
type Topic struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	Replies int    `json:"replies"`
	URL     string `json:"url"`
	Content string `json:"content"`
	Node    struct {
		Name string `json:"name"`
	} `json:"node"`
	Created int64 `json:"created"`
}

// TopicsByNode fetches topics for a given node.
// API: GET /api/topics/show.json?node_name={node}
func (c *Client) TopicsByNode(ctx context.Context, node string) ([]model.NewsItem, error) {
	endpoint := fmt.Sprintf("%s/api/topics/show.json", c.baseURL)
	q := url.Values{"node_name": {node}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("v2ex: status %d", resp.StatusCode)
	}
	var raw []Topic
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	items := make([]model.NewsItem, 0, len(raw))
	for _, t := range raw {
		urlStr := t.URL
		if urlStr == "" {
			urlStr = fmt.Sprintf("%s/t/%d", c.baseURL, t.ID)
		}
		items = append(items, model.NewsItem{
			ID:        fmt.Sprintf("%d", t.ID),
			Title:     t.Title,
			URL:       urlStr,
			NodeName:  t.Node.Name,
			Replies:   t.Replies,
			CreatedAt: time.Unix(t.Created, 0),
			Content:   t.Content,
		})
	}
	return items, nil
}

// NodeMeta represents minimal node metadata we care about.
type NodeMeta struct {
	Name  string `json:"name"`
	Title string `json:"title"`
}

// NodeTitle fetches the human-friendly title for a node by its slug (name).
// API: GET /api/v2/nodes/:node_name
func (c *Client) NodeTitle(ctx context.Context, node string) (string, error) {
	endpoint := fmt.Sprintf("%s/api/v2/nodes/%s", c.baseURL, url.PathEscape(node))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("v2ex: node status %d", resp.StatusCode)
	}
	// Support both bare object and {"result": {...}} envelope forms.
	var envelope struct {
		Result *NodeMeta `json:"result"`
		Name   string    `json:"name"`
		Title  string    `json:"title"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return "", err
	}
	if envelope.Result != nil {
		if envelope.Result.Title != "" {
			return envelope.Result.Title, nil
		}
		return envelope.Result.Name, nil
	}
	if envelope.Title != "" {
		return envelope.Title, nil
	}
	return envelope.Name, nil
}
