package worker

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"quaily-journalist/internal/ai"
	"quaily-journalist/internal/model"
	"quaily-journalist/internal/newsletter"
	"quaily-journalist/internal/quaily"
	"quaily-journalist/internal/storage"
)

type NewsletterBuilder struct {
	Store         *storage.RedisStore
	Source        string
	Channel       string
	Frequency     string
	TopN          int
	MinItems      int
	OutputDir     string
	Interval      time.Duration // how often to evaluate/publish
	Nodes         []string
	SkipDuration  time.Duration
	Preface       string
	Postscript    string
	BaseURL       string // for node links
	Language      string
	Summarizer    ai.Summarizer
	TitleTemplate string
	Quaily        *quaily.Client
}

func (w *NewsletterBuilder) Start(ctx context.Context) error {
	if w.Interval <= 0 {
		w.Interval = 30 * time.Minute
	}
	// ensure base/channel directory exists
	channelDir := filepath.Join(w.OutputDir, w.Channel)
	if err := os.MkdirAll(channelDir, 0o755); err != nil {
		return err
	}
	// run immediately then on interval
	w.runOnce(ctx)

	t := time.NewTicker(w.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			w.runOnce(ctx)
		}
	}
}

func (w *NewsletterBuilder) runOnce(ctx context.Context) {
	period := periodKey(w.Frequency, time.Now().UTC())
	published, err := w.Store.IsPublished(ctx, w.Channel, period)
	if err != nil {
		log.Printf("builder: check published err=%v", err)
		return
	}
	if published {
		return
	}

	// Fetch more than TopN so filtering by nodes still leaves enough.
	fetchN := w.TopN * 5
	if fetchN < w.TopN { // overflow safety, though unlikely
		fetchN = w.TopN
	}
	items, err := w.Store.TopNews(ctx, w.Source, period, fetchN)
	if err != nil {
		log.Printf("builder: fetch top news err=%v", err)
		return
	}
	// For Hacker News, nodes represent lists to poll; only filter by nodes if
	// they include item types (ask/show/job/story). Otherwise, skip filtering.
	if strings.ToLower(w.Source) == "hackernews" {
		items = filterHNTypes(items, w.Nodes)
	} else {
		items = filterByNodes(items, w.Nodes)
	}
	// filter out low-signal items (safety, though collector already skips)
	nz := make([]model.WithScore, 0, len(items))
	for _, ws := range items {
		if strings.ToLower(w.Source) == "hackernews" {
			if ws.Score > 0 { // use computed score only; comments may be 0
				nz = append(nz, ws)
			}
		} else {
			if ws.Item.Replies > 0 && ws.Score > 0 {
				nz = append(nz, ws)
			}
		}
	}
	items = nz
	// filter by skip marks
	filtered := make([]model.WithScore, 0, len(items))
	for _, ws := range items {
		skip, err := w.Store.IsSkipped(ctx, w.Channel, ws.Item.ID)
		if err != nil {
			log.Printf("builder: skip-check err id=%s err=%v", ws.Item.ID, err)
			continue
		}
		if !skip {
			filtered = append(filtered, ws)
		}
	}
	items = filtered
	if len(items) < w.MinItems {
		return
	}
	md := w.renderMarkdown(period, items)
	name := w.filename(period)
	path := filepath.Join(w.OutputDir, w.Channel, name)
	if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
		log.Printf("builder: write file err=%v", err)
		return
	}
	if err := w.Store.MarkPublished(ctx, w.Channel, period); err != nil {
		log.Printf("builder: mark published err=%v", err)
		return
	}
	// mark items as skipped for the configured duration
	for _, ws := range items[:min(len(items), w.TopN)] {
		if err := w.Store.MarkSkipped(ctx, w.Channel, ws.Item.ID, w.SkipDuration); err != nil {
			log.Printf("builder: mark skipped err id=%s err=%v", ws.Item.ID, err)
		}
	}
	log.Printf("builder: published %s with %d items", path, len(items))
	// After generating, publish to Quaily if configured
	if w.Quaily != nil {
		ctxPub, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := quaily.PublishMarkdownFile(ctxPub, w.Quaily, path, w.Channel); err != nil {
			log.Printf("builder: quaily publish failed: %v", err)
		} else {
			log.Printf("builder: quaily publish ok for %s", path)
		}
	}
}

func (w *NewsletterBuilder) filename(period string) string {
	// Always use ":frequency-YYYYMMDD.md" as filename
	dateName := time.Now().UTC().Format("20060102")
	return fmt.Sprintf("%s-%s.md", strings.ToLower(w.Frequency), dateName)
}

func (w *NewsletterBuilder) renderMarkdown(period string, items []model.WithScore) string {
	// Build template data
	// Determine post title: use configured template or default to "Digest of <Channel> <YYYY-MM-DD>"
	postTitle := strings.TrimSpace(w.TitleTemplate)
	if postTitle == "" {
		postTitle = fmt.Sprintf("Digest of %s %s", w.Channel, time.Now().UTC().Format("2006-01-02"))
	}
	// Slug is always the filename without ".md"
	name := w.filename(period)
	slug := strings.TrimSuffix(name, ".md")
	data := newsletter.Data{
		Title:      postTitle,
		Slug:       slug,
		Datetime:   time.Now().UTC().Format("2006-01-02 15:04"),
		Preface:    w.Preface,
		Postscript: w.Postscript,
		Items:      make([]newsletter.Item, 0, min(len(items), w.TopN)),
	}
	// Use a base context and rely on per-call timeouts inside the AI client
	ctxAI := context.Background()
	maxN := min(len(items), w.TopN)
	// Resolve node display titles via cached values in storage (populated at init).
	nodeTitle := map[string]string{}
	set := map[string]struct{}{}
	for i := 0; i < maxN; i++ {
		set[items[i].Item.NodeName] = struct{}{}
	}
	for n := range set {
		if t, err := w.Store.GetNodeTitle(context.Background(), w.Source, n); err == nil && strings.TrimSpace(t) != "" {
			nodeTitle[n] = t
		}
	}
	for i := 0; i < maxN; i++ {
		it := items[i].Item
		var desc string
		if w.Summarizer != nil {
			if d, err := w.Summarizer.SummarizeItem(ctxAI, it.Title, it.Content, w.Language); err == nil && d != "" {
				desc = d
			}
		}
		nodeURL := nodeURLFor(w.Source, w.BaseURL, it.NodeName)
		displayNode := it.NodeName
		if t, ok := nodeTitle[it.NodeName]; ok && strings.TrimSpace(t) != "" {
			displayNode = t
		}
		data.Items = append(data.Items, newsletter.Item{
			Title:       it.Title,
			URL:         it.URL,
			NodeName:    displayNode,
			NodeURL:     nodeURL,
			Description: desc,
			Replies:     it.Replies,
			Created:     it.CreatedAt.UTC().Format("2006-01-02 15:04"),
		})
	}
	// Post-level summary: prefer AI, fallback to heuristic to ensure non-empty
	raw := make([]model.NewsItem, 0, maxN)
	for i := 0; i < maxN; i++ {
		raw = append(raw, items[i].Item)
	}
	if w.Summarizer != nil {
		if s, err := w.Summarizer.SummarizePost(ctxAI, raw, w.Language); err == nil {
			data.Summary = strings.TrimSpace(s)
		}
	}
	if strings.TrimSpace(data.Summary) == "" {
		// Fallback summary built from titles if AI not configured or returned empty
		titles := make([]string, 0, min(3, len(raw)))
		for i := 0; i < min(3, len(raw)); i++ {
			titles = append(titles, raw[i].Title)
		}
		if len(titles) > 0 {
			data.Summary = fmt.Sprintf("Top highlights: %s.", strings.Join(titles, ", "))
		}
	}
	out, err := newsletter.Render(data)
	if err != nil {
		log.Printf("builder: render template err=%v", err)
		return ""
	}
	if !utf8.ValidString(out) {
		out = string([]rune(out))
	}
	return out
}

// no local summary fallback; descriptions remain empty when AI is not configured

func filterByNodes(items []model.WithScore, nodes []string) []model.WithScore {
	if len(nodes) == 0 {
		return items
	}
	set := map[string]struct{}{}
	for _, n := range nodes {
		set[strings.TrimSpace(strings.ToLower(n))] = struct{}{}
	}
	out := make([]model.WithScore, 0, len(items))
	for _, it := range items {
		if _, ok := set[strings.ToLower(it.Item.NodeName)]; ok {
			out = append(out, it)
		}
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// nodeURLFor returns a source-appropriate URL for a node/category name.
func nodeURLFor(source, baseURL, node string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	base := strings.TrimRight(baseURL, "/")
	switch source {
	case "v2ex":
		return base + "/go/" + node
	case "hackernews":
		// Map HN types to list pages for convenience.
		n := strings.ToLower(strings.TrimSpace(node))
		switch n {
		case "ask":
			return base + "/ask"
		case "show":
			return base + "/show"
		case "job", "jobs":
			return base + "/jobs"
		default:
			return base + "/news"
		}
	default:
		return base
	}
}

// filterHNTypes filters only when nodes include known HN item types; otherwise returns input unmodified.
func filterHNTypes(items []model.WithScore, nodes []string) []model.WithScore {
	if len(nodes) == 0 {
		return items
	}
	// Determine which types are specified in nodes
	allowed := map[string]struct{}{}
	for _, n := range nodes {
		s := strings.ToLower(strings.TrimSpace(n))
		switch s {
		case "ask", "show", "job", "story":
			allowed[s] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		// nodes likely specify lists (top/new/best/ask/show/job); do not filter here
		return items
	}
	out := make([]model.WithScore, 0, len(items))
	for _, it := range items {
		if _, ok := allowed[strings.ToLower(it.Item.NodeName)]; ok {
			out = append(out, it)
		}
	}
	return out
}
