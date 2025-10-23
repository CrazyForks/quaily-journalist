package hackernews

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"quaily-journalist/internal/model"
)

// Client is a minimal Hacker News API client.
// Docs: https://github.com/HackerNews/API
type Client struct {
	baseAPI string
	client  *http.Client
}

// NewClient creates a new Hacker News client. baseAPI should be something like
// "https://hacker-news.firebaseio.com/v0". If empty, it defaults to the v0 endpoint.
func NewClient(baseAPI string) *Client {
	if strings.TrimSpace(baseAPI) == "" {
		baseAPI = "https://hacker-news.firebaseio.com/v0"
	}
	return &Client{
		baseAPI: strings.TrimRight(baseAPI, "/"),
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// hnItem mirrors the subset of HN item fields we care about.
type hnItem struct {
	ID          int    `json:"id"`
	Type        string `json:"type"` // story, job, ask, show, poll, etc.
	By          string `json:"by"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Text        string `json:"text"`
	Time        int64  `json:"time"`
	Kids        []int  `json:"kids"`
	Descendants int    `json:"descendants"`
	Score       int    `json:"score"`
	Parts       []int  `json:"parts"` // polls
}

// TopStories returns top stories as NewsItems (up to limit).
func (c *Client) TopStories(ctx context.Context, limit int) ([]model.NewsItem, error) {
	return c.storiesByList(ctx, "topstories", limit)
}

// NewStories returns new stories as NewsItems (up to limit).
func (c *Client) NewStories(ctx context.Context, limit int) ([]model.NewsItem, error) {
	return c.storiesByList(ctx, "newstories", limit)
}

// BestStories returns best stories as NewsItems (up to limit).
func (c *Client) BestStories(ctx context.Context, limit int) ([]model.NewsItem, error) {
	return c.storiesByList(ctx, "beststories", limit)
}

// AskStories returns Ask HN posts (up to limit).
func (c *Client) AskStories(ctx context.Context, limit int) ([]model.NewsItem, error) {
	return c.storiesByList(ctx, "askstories", limit)
}

// ShowStories returns Show HN posts (up to limit).
func (c *Client) ShowStories(ctx context.Context, limit int) ([]model.NewsItem, error) {
	return c.storiesByList(ctx, "showstories", limit)
}

// JobStories returns job posts (up to limit).
func (c *Client) JobStories(ctx context.Context, limit int) ([]model.NewsItem, error) {
	return c.storiesByList(ctx, "jobstories", limit)
}

// Item fetches a single HN item by ID and converts it into NewsItem.
func (c *Client) Item(ctx context.Context, id int) (model.NewsItem, error) {
	var zero model.NewsItem
	endpoint := fmt.Sprintf("%s/item/%d.json", c.baseAPI, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return zero, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return zero, fmt.Errorf("hackernews: item %d status %d", id, resp.StatusCode)
	}
	var it hnItem
	if err := json.NewDecoder(resp.Body).Decode(&it); err != nil {
		return zero, err
	}
	return convertItem(it), nil
}

// storiesByList fetches IDs from a stories list and resolves them to NewsItems.
func (c *Client) storiesByList(ctx context.Context, list string, limit int) ([]model.NewsItem, error) {
	ids, err := c.fetchIDs(ctx, list)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	slog.Info("hackernews: fetching items", "list", list, "count", len(ids))
	return c.itemsByIDs(ctx, ids)
}

// fetchIDs loads a list endpoint such as topstories/newstories/etc.
func (c *Client) fetchIDs(ctx context.Context, list string) ([]int, error) {
	path := fmt.Sprintf("%s/%s.json", c.baseAPI, url.PathEscape(list))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("hackernews: %s status %d", list, resp.StatusCode)
	}
	var ids []int
	if err := json.NewDecoder(resp.Body).Decode(&ids); err != nil {
		return nil, err
	}
	return ids, nil
}

// itemsByIDs resolves multiple IDs concurrently into NewsItems.
func (c *Client) itemsByIDs(ctx context.Context, ids []int) ([]model.NewsItem, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	// bounded concurrency
	const maxWorkers = 8
	type result struct {
		idx  int
		item model.NewsItem
		err  error
	}
	out := make([]result, len(ids))
	sem := make(chan struct{}, maxWorkers)
	done := make(chan result, len(ids))
	for i, id := range ids {
		i, id := i, id
		sem <- struct{}{}
		go func() {
			defer func() { <-sem }()
			// Per-item timeout to avoid hanging
			ictx, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()
			it, err := c.Item(ictx, id)
			done <- result{idx: i, item: it, err: err}
		}()
	}
	// wait for all
	for i := 0; i < len(ids); i++ {
		r := <-done
		if r.err != nil {
			// skip failed ones silently; continue
			continue
		}
		out[r.idx] = r
	}
	// collect non-zero entries preserving order
	items := make([]model.NewsItem, 0, len(ids))
	for _, r := range out {
		if r.item.ID != "" {
			items = append(items, r.item)
		}
	}
	return items, nil
}

// convertItem maps an hnItem to our NewsItem model.
func convertItem(h hnItem) model.NewsItem {
	idStr := fmt.Sprintf("%d", h.ID)
	urlStr := strings.TrimSpace(h.URL)
	if urlStr == "" {
		urlStr = "https://news.ycombinator.com/item?id=" + idStr
	}
	content := stripHTML(h.Text)
	// Derive a pseudo-node for filtering: ask/show/job/story
	typ := strings.ToLower(strings.TrimSpace(h.Type))
	cat := typ
	if typ == "story" {
		t := strings.ToLower(strings.TrimSpace(h.Title))
		if strings.HasPrefix(t, "ask hn:") {
			cat = "ask"
		} else if strings.HasPrefix(t, "show hn:") {
			cat = "show"
		} else {
			cat = "story"
		}
	} else if typ == "job" {
		cat = "job"
	}
	return model.NewsItem{
		ID:        idStr,
		Title:     h.Title,
		URL:       urlStr,
		NodeName:  cat,
		Replies:   maxInt(h.Descendants, len(h.Kids)),
		Points:    h.Score,
		CreatedAt: time.Unix(h.Time, 0),
		Content:   content,
	}
}

var htmlTagRe = regexp.MustCompile(`<[^>]+>`) // best-effort removal

func stripHTML(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	// Remove common HTML tags to feed cleaner text to summarizers.
	// This is a minimal approach; HN "text" is simple HTML.
	s = htmlTagRe.ReplaceAllString(s, "")
	// Unescape a few common entities by hand to avoid extra deps.
	replacer := strings.NewReplacer(
		"&quot;", "\"",
		"&apos;", "'",
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
	)
	return strings.TrimSpace(replacer.Replace(s))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
