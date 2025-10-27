package scrape

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// CloudflareClient calls Cloudflare Browser Rendering REST API.
// See: https://developers.cloudflare.com/browser-rendering/rest-api/
type CloudflareClient struct {
	baseURL string
	token   string
	http    *http.Client
	timeout time.Duration
}

type markdownRequest struct {
	URL                  string   `json:"url"`
	RejectRequestPattern []string `json:"rejectRequestPattern,omitempty"`
}

type scrapeRequest struct {
	URL      string             `json:"url"`
	Elements []scrapeRequestEle `json:"elements"`
}

type scrapeRequestEle struct {
	Selector string `json:"selector"`
}

// Common response shapes observed/expected.
type markdownResponse struct {
	Success bool   `json:"success"`
	Result  string `json:"result"`
	Errors  any    `json:"errors"`
}

type scrapeResponse struct {
	Success bool               `json:"success"`
	Result  []scrapeResultItem `json:"result"`
	Errors  any                `json:"errors"`
}
type scrapeResultItem struct {
	Results  []scrapeResultItemEle `json:"results"`
	Selector string                `json:"selector"`
}
type scrapeResultItemEle struct {
	Html string `json:"html"`
	Text string `json:"text"`
}

// NewCloudflare creates a new client from an account ID.
// Endpoint: https://api.cloudflare.com/client/v4/accounts/<ACCOUNT_ID>/browser-rendering/markdown
func NewCloudflare(accountID, token string, timeout time.Duration) *CloudflareClient {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	baseURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/browser-rendering", strings.TrimSpace(accountID))
	return &CloudflareClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: timeout},
		timeout: timeout,
	}
}

// Scrape fetches title and content for a URL using Cloudflare Browser Rendering.
func (c *CloudflareClient) Scrape(ctx context.Context, u string) (title, content string, err error) {
	body, _ := json.Marshal(markdownRequest{
		URL:                  u,
		RejectRequestPattern: []string{"/^.*\\.(css)/"},
	})
	slog.Info("cloudflare: markdown request", "url", u)
	r, err := c.scrape(ctx, "/markdown", u, body)
	if err != nil {
		return "", "", err
	}

	slog.Info("cloudflare: markdown response", "body", string(r))
	var envelope markdownResponse
	if err := json.Unmarshal(r, &envelope); err != nil {
		return "", "", err
	}

	content = envelope.Result
	// try to find the first title in the markdown content
	// the title will start with `# ` or `## ` ...
	lines := strings.Split(content, "\n")
	possibleTitles := make([]string, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			possibleTitles = append(possibleTitles, line)
		}
	}
	if len(possibleTitles) != 0 {
		// sort possible titles by how many `#` they have (less is better)
		// so that `# Title` is preferred over `## Title`
		// and `## Title` is preferred over `### Title`
		// etc.
		// use Slice sorting
		sort.Slice(possibleTitles, func(i, j int) bool {
			return strings.Count(possibleTitles[i], "#") < strings.Count(possibleTitles[j], "#")
		})
		title = strings.TrimSpace(strings.TrimLeft(possibleTitles[0], "#"))
	}

	if title == "" {
		body, _ = json.Marshal(scrapeRequest{
			URL: u,
			Elements: []scrapeRequestEle{
				{Selector: "title"},
			},
		})
		r, err = c.scrape(ctx, "/scrape", u, body)
		if err != nil {
			return "", content, err
		}
		var scrapeEnv scrapeResponse
		if err := json.Unmarshal(r, &scrapeEnv); err != nil {
			return "", content, err
		}
		if len(scrapeEnv.Result) > 0 && len(scrapeEnv.Result[0].Results) > 0 {
			scrapedTitle := strings.TrimSpace(scrapeEnv.Result[0].Results[0].Text)
			if scrapedTitle != "" {
				title = scrapedTitle
			}
		}
	}

	return title, content, nil
}

func (c *CloudflareClient) scrape(ctx context.Context, path, u string, body []byte) (raw []byte, err error) {
	if c == nil {
		return nil, errors.New("nil cloudflare client")
	}
	if _, err := url.ParseRequestURI(u); err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var b []byte
	b, _ = io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cloudflare scrape failed: status=%d body=%s", resp.StatusCode, string(b))
	}
	return b, nil
}
