package scrape

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// Common response shapes observed/expected.
type scrapeResponse struct {
	Success bool   `json:"success"`
	Result  string `json:"result"`
	Errors  any    `json:"errors"`
}

// NewCloudflare creates a new client from an account ID.
// Endpoint: https://api.cloudflare.com/client/v4/accounts/<ACCOUNT_ID>/browser-rendering/markdown
func NewCloudflare(accountID, token string, timeout time.Duration) *CloudflareClient {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	baseURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/browser-rendering/markdown", strings.TrimSpace(accountID))
	return &CloudflareClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: timeout},
		timeout: timeout,
	}
}

// Scrape fetches title and content for a URL using Cloudflare Browser Rendering.
func (c *CloudflareClient) Scrape(ctx context.Context, u string) (title, content string, err error) {
	if c == nil {
		return "", "", errors.New("nil cloudflare client")
	}
	if _, err := url.ParseRequestURI(u); err != nil {
		return "", "", fmt.Errorf("invalid url: %w", err)
	}
	body, _ := json.Marshal(markdownRequest{
		URL:                  u,
		RejectRequestPattern: []string{"/^.*\\.(css)/"},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("cloudflare scrape failed: status=%d body=%s", resp.StatusCode, string(b))
	}
	var envelope scrapeResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
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
	if len(possibleTitles) == 0 {
		// no title found
		return "", content, nil
	}
	// sort possible titles by how many `#` they have (less is better)
	// so that `# Title` is preferred over `## Title`
	// and `## Title` is preferred over `### Title`
	// etc.
	// use Slice sorting
	sort.Slice(possibleTitles, func(i, j int) bool {
		return strings.Count(possibleTitles[i], "#") < strings.Count(possibleTitles[j], "#")
	})
	title = strings.TrimSpace(strings.TrimLeft(possibleTitles[0], "#"))

	return title, content, nil
}
