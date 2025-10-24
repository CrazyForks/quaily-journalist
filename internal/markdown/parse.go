package markdown

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Document represents a Markdown file with YAML frontmatter.
type Document struct {
	Frontmatter map[string]any
	Body        string
}

// ParseFile reads a Markdown file and extracts YAML frontmatter and body.
// Frontmatter is expected at the top of the file between two lines containing only "---".
func ParseFile(path string) (Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return Document{}, err
	}
	defer f.Close()

	br := bufio.NewReader(f)
	peek, err := br.Peek(3)
	if err != nil && !errors.Is(err, io.EOF) {
		return Document{}, err
	}
	var hasFM bool
	if string(peek) == "---" {
		hasFM = true
	}
	var fmBuf strings.Builder
	var bodyBuf strings.Builder

	if hasFM {
		// Consume first line '---' fully
		line, err := br.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return Document{}, err
		}
		_ = line // discard
		// Read until next line starting with '---' (exact match)
		for {
			l, err := br.ReadString('\n')
			if err != nil && !errors.Is(err, io.EOF) {
				return Document{}, err
			}
			trim := strings.TrimSpace(l)
			if trim == "---" {
				break
			}
			fmBuf.WriteString(l)
			if errors.Is(err, io.EOF) {
				break
			}
		}
	}
	// The rest is body
	for {
		l, err := br.ReadString('\n')
		bodyBuf.WriteString(l)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Document{}, err
		}
	}

	d := Document{
		Frontmatter: map[string]any{},
		Body:        bodyBuf.String(),
	}

	if hasFM {
		m := map[string]any{}
		if err := yaml.Unmarshal([]byte(fmBuf.String()), &m); err != nil {
			return Document{}, err
		}
		d.Frontmatter = m
	}
	return d, nil
}
