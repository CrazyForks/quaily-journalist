package markdown

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseWithFrontmatter(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "post.md")
	content := "" +
		"---\n" +
		"title: \"Hacker News Daily YYYY-MM-DD\"\n" +
		"slug: daily-20251024\n" +
		"datetime: 2025-10-24 00:30\n" +
		"summary: |-\n" +
		"  Some summary here.\n" +
		"---\n\n" +
		"## [A Title](https://example.com)\n\nBody paragraph here.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	doc, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(doc.Frontmatter) == 0 {
		t.Fatalf("expected frontmatter, got empty")
	}
	// Required keys
	if _, ok := doc.Frontmatter["title"]; !ok {
		t.Errorf("missing title in frontmatter")
	}
	if _, ok := doc.Frontmatter["slug"]; !ok {
		t.Errorf("missing slug in frontmatter")
	}
	if _, ok := doc.Frontmatter["datetime"]; !ok {
		t.Errorf("missing datetime in frontmatter")
	}
	if _, ok := doc.Frontmatter["summary"]; !ok {
		t.Errorf("missing summary in frontmatter")
	}
	b := doc.Body
	if b == "" {
		t.Fatalf("expected non-empty body")
	}
	if wantSub := "## [A Title](https://example.com)"; !contains(b, wantSub) {
		t.Errorf("body missing expected substring %q; got: %q", wantSub, b)
	}
}

func TestParseWithoutFrontmatter(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "no_fm.md")
	body := "# Hello\n\nNo frontmatter here.\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	doc, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(doc.Frontmatter) != 0 {
		t.Fatalf("expected empty frontmatter, got: %+v", doc.Frontmatter)
	}
	if doc.Body != body {
		t.Errorf("body mismatch.\nwant: %q\n got: %q", body, doc.Body)
	}
}

// contains is a simple substring helper without importing strings,
// to keep the test focused on parser behavior.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
