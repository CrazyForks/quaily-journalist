package quaily

import (
	"context"
	"fmt"
	"os"

	"quaily-journalist/internal/markdown"
)

// DeliverMarkdownOrSlug delivers a post either by parsing a markdown file to
// obtain its frontmatter slug, or directly using the provided slug.
// If pathOrSlug points to an existing file, it is treated as a markdown path.
// Otherwise, it is treated as a slug.
func DeliverMarkdownOrSlug(ctx context.Context, c *Client, pathOrSlug, channelSlug string) error {
	if _, err := os.Stat(pathOrSlug); err == nil {
		// Treat as markdown file
		doc, err := markdown.ParseFile(pathOrSlug)
		if err != nil {
			return fmt.Errorf("read markdown: %w", err)
		}
		v, ok := doc.Frontmatter["slug"]
		if !ok {
			return fmt.Errorf("frontmatter missing 'slug' in %s", pathOrSlug)
		}
		slug, ok := v.(string)
		if !ok || slug == "" {
			return fmt.Errorf("frontmatter 'slug' must be a non-empty string in %s", pathOrSlug)
		}
		return c.DeliverPost(ctx, channelSlug, slug)
	}
	// Not a file; assume it's a post slug directly
	return c.DeliverPost(ctx, channelSlug, pathOrSlug)
}

