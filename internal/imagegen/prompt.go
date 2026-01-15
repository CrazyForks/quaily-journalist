package imagegen

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// PromptData contains inputs for building a cover prompt.
type PromptData struct {
	Title       string
	Summary     string
	Highlights  []string
	Language    string
	AspectRatio string
}

const defaultPrompt = `Create a clean, modern infographic cover image for a news digest.

Requirements:
- Aspect ratio: %s (widescreen).
- Language for any text: %s.
- Title: "%s".
- Subtitle/summary: "%s".
- Highlights: %s.
- Style: flat vector, high-contrast palette, simple shapes, no photos, no logos, no watermarks.
- Keep text minimal, aligned, and clearly legible.`

// BuildCoverPrompt builds a prompt from data, using template if provided.
// Template variables: {Title}, {Summary}, {Highlights}, {Language}, {AspectRatio}
func BuildCoverPrompt(d PromptData, template string) string {
	title := strings.TrimSpace(d.Title)
	if title == "" {
		title = "Daily Digest"
	}
	summary := strings.TrimSpace(d.Summary)
	if summary == "" {
		summary = "Top stories and themes from today."
	}
	lang := strings.TrimSpace(d.Language)
	if lang == "" {
		lang = "English"
	}
	aspect := strings.TrimSpace(d.AspectRatio)
	if aspect == "" {
		aspect = "16:9"
	}
	highlights := cleanHighlights(d.Highlights, 5, 80)
	hl := strings.Join(highlights, "; ")
	if hl == "" {
		hl = "Key highlights from today"
	}

	if strings.TrimSpace(template) == "" {
		return fmt.Sprintf(defaultPrompt, aspect, lang, title, summary, hl)
	}
	replacer := strings.NewReplacer(
		"{Title}", title,
		"{Summary}", summary,
		"{Highlights}", hl,
		"{Language}", lang,
		"{AspectRatio}", aspect,
	)
	return replacer.Replace(template)
}

func cleanHighlights(items []string, maxItems, maxLen int) []string {
	out := make([]string, 0, min(len(items), maxItems))
	for _, it := range items {
		t := strings.TrimSpace(it)
		if t == "" {
			continue
		}
		if maxLen > 0 && utf8.RuneCountInString(t) > maxLen {
			t = truncateRunes(t, maxLen-3) + "..."
		}
		out = append(out, t)
		if len(out) >= maxItems {
			break
		}
	}
	return out
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
