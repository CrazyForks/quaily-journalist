package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"quaily-journalist/internal/model"
	"quaily-journalist/internal/newsletter"
	"quaily-journalist/internal/redisclient"
	"quaily-journalist/internal/storage"

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
			Name       string
			Source     string
			Frequency  string
			TopN       int
			MinItems   int
			OutputDir  string
			Nodes      []string
			Preface    string
			Postscript string
		}
		for i := range cfg.Newsletters.Channels {
			c := cfg.Newsletters.Channels[i]
			if c.Name == channelName {
				ch = &struct {
					Name       string
					Source     string
					Frequency  string
					TopN       int
					MinItems   int
					OutputDir  string
					Nodes      []string
					Preface    string
					Postscript string
				}{
					Name:       c.Name,
					Source:     strings.ToLower(c.Source),
					Frequency:  strings.ToLower(c.Frequency),
					TopN:       c.TopN,
					MinItems:   c.MinItems,
					OutputDir:  c.OutputDir,
					Nodes:      c.Nodes,
					Preface:    c.Preface,
					Postscript: c.Postscript,
				}
				break
			}
		}
		if ch == nil {
			return fmt.Errorf("channel not found: %s", channelName)
		}

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

		items, err := store.TopNews(ctx, ch.Source, period, fetchN)
		if err != nil {
			return err
		}
		items = filterByNodesLocal(items, ch.Nodes)
		// ensure zero-reply and zero-score items are excluded
		nz := make([]model.WithScore, 0, len(items))
		for _, ws := range items {
			if ws.Item.Replies > 0 && ws.Score > 0 {
				nz = append(nz, ws)
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
		postTitle := fmt.Sprintf("%s — %s", ch.Name, period)
		// Filename and slug: frequency-YYYYMMDD.md
		dateName := time.Now().UTC().Format("20060102")
		fileName := fmt.Sprintf("%s-%s.md", ch.Frequency, dateName)
		slug := strings.TrimSuffix(fileName, ".md")
		baseURL := cfg.Sources.V2EX.BaseURL // only v2ex supported currently
		nd := newsletter.Data{
			Title:      postTitle,
			Slug:       slug,
			Datetime:   time.Now().UTC().Format("2006-01-02 15:04"),
			Preface:    ch.Preface,
			Postscript: ch.Postscript,
			Items:      make([]newsletter.Item, 0, len(items)),
		}
		for _, ws := range items {
			it := ws.Item
			nodeURL := strings.TrimRight(baseURL, "/") + "/go/" + it.NodeName
			nd.Items = append(nd.Items, newsletter.Item{
				Title:       it.Title,
				URL:         it.URL,
				NodeName:    it.NodeName,
				NodeURL:     nodeURL,
				Description: summarizeLocal(it),
				Replies:     it.Replies,
				Created:     it.CreatedAt.UTC().Format("2006-01-02 15:04"),
			})
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

// renderMarkdownLocal produces a simple markdown similar to builder.
func escapeMDLocal(s string) string {
	replacer := strings.NewReplacer("[", "\\[", "]", "\\]", "(", "\\(", ")", "\\)")
	return replacer.Replace(s)
}

func summarizeLocal(it model.NewsItem) string {
	text := strings.TrimSpace(it.Content)
	if text == "" {
		text = it.Title
	}
	text = strings.ReplaceAll(text, "\n", " ")
	r := []rune(text)
	if len(r) > 200 {
		text = string(r[:200]) + "…"
	}
	return text
}
