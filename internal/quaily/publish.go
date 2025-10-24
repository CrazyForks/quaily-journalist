package quaily

import (
	"context"
	"fmt"
	"time"

	"quaily-journalist/internal/markdown"
)

// PublishMarkdownFile parses a Markdown file, uses its frontmatter as params,
// adds channel_slug and content, creates the post and publishes it.
func PublishMarkdownFile(ctx context.Context, c *Client, path, channelSlug string) error {
	doc, err := markdown.ParseFile(path)
	if err != nil {
		return fmt.Errorf("read markdown: %w", err)
	}
	params := map[string]any{}
	for k, v := range doc.Frontmatter {
		params[k] = v
	}
	params["channel_slug"] = channelSlug
	params["content"] = doc.Body
	// for the datetime
	// if it's not RFC3339, try to parse it as RFC 3339
	// re-format it as RFC3339 if it's present
	if dtRaw, ok := params["datetime"]; ok {
		if dtStr, ok := dtRaw.(string); ok {
			if t, err := time.Parse("2006-01-02 15:04", dtStr); err == nil {
				params["datetime"] = t.Format(time.RFC3339)
			}
		}
	}
	params["content"] = doc.Body

	postID, err := c.CreatePost(ctx, channelSlug, params)
	if err != nil {
		return err
	}
	return c.PublishPost(ctx, channelSlug, postID)
}
