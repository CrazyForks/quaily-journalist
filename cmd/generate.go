package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"quaily-journalist/internal/ai"
	"quaily-journalist/internal/model"
	"quaily-journalist/internal/newsletter"
	"quaily-journalist/internal/redisclient"
	"quaily-journalist/internal/storage"
	"quaily-journalist/internal/v2ex"

	"github.com/spf13/cobra"
)

// generateCmd force-generates a newsletter for a given channel, ignoring skip/published state.
var generateCmd = &cobra.Command{
	Use:   "generate <channel>",
	Short: "Force-generate a newsletter for a channel (daily)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		channelName := args[0]
		cfg := GetConfig()

		// find channel
		var ch *struct {
			Name      string
			Source    string
			Frequency string
			TopN      int
			MinItems  int
			OutputDir string
			Nodes     []string
			Template  struct {
				Title      string
				Preface    string
				Postscript string
			}
			Language string
		}
		for i := range cfg.Newsletters.Channels {
			c := cfg.Newsletters.Channels[i]
			if c.Name == channelName {
				ch = &struct {
					Name      string
					Source    string
					Frequency string
					TopN      int
					MinItems  int
					OutputDir string
					Nodes     []string
					Template  struct {
						Title      string
						Preface    string
						Postscript string
					}
					Language string
				}{
					Name:      c.Name,
					Source:    strings.ToLower(c.Source),
					Frequency: strings.ToLower(c.Frequency),
					TopN:      c.TopN,
					MinItems:  c.MinItems,
					OutputDir: cfg.Newsletters.OutputDir,
					Nodes:     c.Nodes,
					Template: struct {
						Title      string
						Preface    string
						Postscript string
					}{
						Title:      c.Template.Title,
						Preface:    c.Template.Preface,
						Postscript: c.Template.Postscript,
					},
					Language: c.Language,
				}
				break
			}
		}
		if ch == nil {
			return fmt.Errorf("channel not found: %s", channelName)
		}

		slog.Info("generate: generating newsletter", "channel", ch.Name, "output", ch.OutputDir)

		// Prepare storage
		rdb := redisclient.New(cfg.Redis)
		defer rdb.Close()
		store := storage.NewRedisStore(rdb)

		// Daily period key (UTC) matches collector storage
		period := time.Now().UTC().Format("2006-01-02")
		// fetch more than TopN to allow node filtering
		fetchN := ch.TopN * 5
		if fetchN < ch.TopN {
			fetchN = ch.TopN
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Prefetch node titles at initialization using the node list from config
		if strings.ToLower(ch.Source) == "v2ex" {
			v2c := v2ex.NewClient(cfg.Sources.V2EX.BaseURL, cfg.Sources.V2EX.Token)
			for _, n := range ch.Nodes {
				n = strings.TrimSpace(n)
				if n == "" {
					continue
				}
				if t, _ := store.GetNodeTitle(context.Background(), "v2ex", n); strings.TrimSpace(t) == "" {
					ctxNode, cancelNode := context.WithTimeout(context.Background(), 5*time.Second)
					if title, err := v2c.NodeTitle(ctxNode, n); err == nil && strings.TrimSpace(title) != "" {
						_ = store.SetNodeTitle(context.Background(), "v2ex", n, title, 30*24*time.Hour)
					}
					cancelNode()
				}
			}
		}

		items, err := store.TopNews(ctx, ch.Source, period, fetchN)
		if err != nil {
			return err
		}
		// For Hacker News, nodes list are lists to poll; only filter by nodes
		// if they include HN item types (ask/show/job/story). Otherwise, skip filtering.
		if ch.Source == "hackernews" {
			items = filterHNTypesLocal(items, ch.Nodes)
		} else {
			items = filterByNodesLocal(items, ch.Nodes)
		}
		// ensure low-signal items are excluded (source-specific)
		nz := make([]model.WithScore, 0, len(items))
		for _, ws := range items {
			if ch.Source == "hackernews" {
				if ws.Score > 0 {
					nz = append(nz, ws)
				}
			} else {
				if ws.Item.Replies > 0 && ws.Score > 0 {
					nz = append(nz, ws)
				}
			}
		}
		items = nz
		if len(items) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No items found for channel; skipping file creation.")
			return nil
		}
		if len(items) < ch.MinItems {
			fmt.Fprintf(cmd.OutOrStdout(), "Only %d items (< min_items=%d); skipping file creation.\n", len(items), ch.MinItems)
			return nil
		}
		if len(items) > ch.TopN {
			items = items[:ch.TopN]
		}

		// Prepare template data
		// Determine post title: use configured template or default to "Digest of <Channel> <YYYY-MM-DD>"
		now := time.Now()
		postTitle := strings.TrimSpace(ch.Template.Title)
		if postTitle == "" {
			postTitle = fmt.Sprintf("Digest of %s %s", ch.Name, period)
		}
		// Expand template variables in configured title/preface/postscript
		postTitle = newsletter.ExpandVars(postTitle, now)
		// Filename and slug: frequency-YYYYMMDD.md
		dateName := time.Now().UTC().Format("20060102")
		fileName := fmt.Sprintf("%s-%s.md", ch.Frequency, dateName)
		slug := strings.TrimSuffix(fileName, ".md")
		var baseURL string
		if ch.Source == "v2ex" {
			baseURL = cfg.Sources.V2EX.BaseURL
		} else if ch.Source == "hackernews" {
			baseURL = "https://news.ycombinator.com"
		} else {
			baseURL = ""
		}
		nd := newsletter.Data{
			Title:      postTitle,
			Slug:       slug,
			Datetime:   time.Now().UTC().Format("2006-01-02 15:04"),
			Preface:    newsletter.ExpandVars(ch.Template.Preface, now),
			Postscript: newsletter.ExpandVars(ch.Template.Postscript, now),
			Items:      make([]newsletter.Item, 0, len(items)),
		}
		// Setup summarizer
		var summarizer ai.Summarizer
		if cfg.OpenAI.APIKey != "" {
			summarizer = ai.NewOpenAI(ai.Config{APIKey: cfg.OpenAI.APIKey, Model: cfg.OpenAI.Model, BaseURL: cfg.OpenAI.BaseURL})
		}
		// Use base context; AI client enforces per-call timeouts
		ctxAI := context.Background()
		// Resolve node titles for display (best-effort) from Redis cache
		titleByNode := map[string]string{}
		set := map[string]struct{}{}
		for _, ws := range items {
			set[ws.Item.NodeName] = struct{}{}
		}
		for n := range set {
			if t, err := store.GetNodeTitle(context.Background(), ch.Source, n); err == nil && strings.TrimSpace(t) != "" {
				titleByNode[n] = t
			}
		}
		for _, ws := range items {
			it := ws.Item
			nodeURL := nodeURLForLocal(ch.Source, baseURL, it.NodeName)
			var desc string
			if summarizer != nil {
				if d, err := summarizer.SummarizeItem(ctxAI, it.Title, it.Content, ch.Language); err == nil && d != "" {
					desc = d
				}
			}
			displayNode := it.NodeName
			if t, ok := titleByNode[it.NodeName]; ok && strings.TrimSpace(t) != "" {
				displayNode = t
			}
			nd.Items = append(nd.Items, newsletter.Item{
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
		raw := make([]model.NewsItem, 0, len(items))
		for _, ws := range items {
			raw = append(raw, ws.Item)
		}
		if summarizer != nil {
			if s, err := summarizer.SummarizePost(ctxAI, raw, ch.Language); err == nil {
				nd.Summary = strings.TrimSpace(s)
			}
			if s, err := summarizer.SummarizePostLikeAZenMaster(ctxAI, raw, ch.Language); err == nil {
				nd.ShortSummary = strings.TrimSpace(s)
			}
		}

		content, err := newsletter.Render(nd)
		if err != nil {
			return err
		}
		if !utf8.ValidString(content) {
			content = string([]rune(content))
		}
		// output path: :output_dir/:channel_name/:frequency-YYYYMMDD.md (overwrite)
		dir := filepath.Join(ch.OutputDir, ch.Name)
		slog.Info("generate: generating newsletter", "channel", ch.Name, "file", filepath.Join(dir, fileName))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		path := filepath.Join(dir, fileName)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Generated: %s\n", path)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(generateCmd)
}

// Local helpers (ignore skip/published)

// filterByNodesLocal filters by node names (case-insensitive).
func filterByNodesLocal(items []model.WithScore, nodes []string) []model.WithScore {
	if len(nodes) == 0 {
		return items
	}
	set := map[string]struct{}{}
	for _, n := range nodes {
		set[strings.ToLower(strings.TrimSpace(n))] = struct{}{}
	}
	out := make([]model.WithScore, 0, len(items))
	for _, ws := range items {
		if _, ok := set[strings.ToLower(ws.Item.NodeName)]; ok {
			out = append(out, ws)
		}
	}
	return out
}

// nodeURLForLocal mirrors worker's logic for building a node/category URL per source
func nodeURLForLocal(source, baseURL, node string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	base := strings.TrimRight(baseURL, "/")
	switch source {
	case "v2ex":
		if base == "" {
			return ""
		}
		return base + "/go/" + node
	case "hackernews":
		if base == "" {
			base = "https://news.ycombinator.com"
		}
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

// filterHNTypesLocal filters only when nodes include known HN item types; otherwise returns input unmodified.
func filterHNTypesLocal(items []model.WithScore, nodes []string) []model.WithScore {
	if len(nodes) == 0 {
		return items
	}
	allowed := map[string]struct{}{}
	for _, n := range nodes {
		s := strings.ToLower(strings.TrimSpace(n))
		switch s {
		case "ask", "show", "job", "story":
			allowed[s] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return items
	}
	out := make([]model.WithScore, 0, len(items))
	for _, ws := range items {
		if _, ok := allowed[strings.ToLower(ws.Item.NodeName)]; ok {
			out = append(out, ws)
		}
	}
	return out
}
