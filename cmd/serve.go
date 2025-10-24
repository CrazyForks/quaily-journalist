package cmd

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"quaily-journalist/internal/ai"
	"quaily-journalist/internal/hackernews"
	"quaily-journalist/internal/redisclient"
	"quaily-journalist/internal/storage"
	"quaily-journalist/internal/v2ex"
	"quaily-journalist/worker"

	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the service workers",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := GetConfig()
		// Redis client
		rdb := redisclient.New(cfg.Redis)
		defer rdb.Close()
		store := storage.NewRedisStore(rdb)

		var collector *worker.V2EXCollector
		var hnCollector *worker.HNCollector

		var nodes []string

		var v2c *v2ex.Client
		var hnc *hackernews.Client

		// V2EX collector setup with union of nodes across channels using v2ex
		if cfg.Sources.V2EX.Token != "" {
			v2c = v2ex.NewClient(cfg.Sources.V2EX.BaseURL, cfg.Sources.V2EX.Token)
			interval, err := time.ParseDuration(cfg.Sources.V2EX.FetchInterval)
			if err != nil {
				return err
			}
			// gather nodes from channels where source==v2ex
			nodeSet := map[string]struct{}{}
			for _, ch := range cfg.Newsletters.Channels {
				if strings.ToLower(ch.Source) == "v2ex" {
					for _, n := range ch.Nodes {
						n = strings.TrimSpace(n)
						if n == "" {
							continue
						}
						nodeSet[n] = struct{}{}
					}
				}
			}
			nodes := make([]string, 0, len(nodeSet))
			for n := range nodeSet {
				nodes = append(nodes, n)
			}
			collector = &worker.V2EXCollector{
				Client:   v2c,
				Store:    store,
				Nodes:    nodes,
				Interval: interval,
			}
		}

		if cfg.Sources.HN.BaseAPI != "" {
			// Hacker News collector setup: use HN channel nodes directly as lists
			hnc = hackernews.NewClient(cfg.Sources.HN.BaseAPI)
			hnInterval, err := time.ParseDuration(cfg.Sources.HN.FetchInterval)
			if err != nil {
				return err
			}
			// Gather union of nodes for HN channels; treat them as lists directly
			hnNodeSet := map[string]struct{}{}
			for _, ch := range cfg.Newsletters.Channels {
				if strings.ToLower(ch.Source) == "hackernews" {
					for _, n := range ch.Nodes {
						n = strings.ToLower(strings.TrimSpace(n))
						if n == "" {
							continue
						}
						hnNodeSet[n] = struct{}{}
					}
				}
			}
			hnLists := make([]string, 0, len(hnNodeSet))
			for n := range hnNodeSet {
				hnLists = append(hnLists, n)
			}
			if len(hnLists) == 0 {
				hnLists = []string{"top"}
			}
			hnCollector = &worker.HNCollector{
				Client:       hnc,
				Store:        store,
				Lists:        hnLists,
				Interval:     hnInterval,
				LimitPerList: 64,
			}
		}

		var summarizer ai.Summarizer
		if cfg.OpenAI.APIKey != "" {
			summarizer = ai.NewOpenAI(ai.Config{APIKey: cfg.OpenAI.APIKey, Model: cfg.OpenAI.Model, BaseURL: cfg.OpenAI.BaseURL})
		}

		// Cache human-friendly node titles at init (best-effort)
		for _, n := range nodes {
			ctxNode, cancelNode := context.WithTimeout(context.Background(), 5*time.Second)
			// Skip fetch if already cached
			if t, _ := store.GetNodeTitle(ctxNode, "v2ex", n); strings.TrimSpace(t) == "" {
				if title, err := v2c.NodeTitle(ctxNode, n); err == nil && strings.TrimSpace(title) != "" {
					_ = store.SetNodeTitle(context.Background(), "v2ex", n, title, 30*24*time.Hour)
				}
			}
			cancelNode()
		}

		// Newsletter builders (one per channel)
		var builders []worker.Worker
		for _, ch := range cfg.Newsletters.Channels {
			sd, err := time.ParseDuration(ch.ItemSkipDuration)
			if err != nil {
				return fmt.Errorf("invalid item_skip_duration for channel %s: %w", ch.Name, err)
			}
			baseURL := cfg.Sources.V2EX.BaseURL
			if strings.ToLower(ch.Source) == "hackernews" {
				baseURL = "https://news.ycombinator.com"
			}
			builders = append(builders, &worker.NewsletterBuilder{
				Store:         store,
				Source:        strings.ToLower(ch.Source),
				Channel:       ch.Name,
				Frequency:     strings.ToLower(ch.Frequency),
				TopN:          ch.TopN,
				MinItems:      ch.MinItems,
				OutputDir:     ch.OutputDir,
				Interval:      30 * time.Minute,
				Nodes:         ch.Nodes,
				SkipDuration:  sd,
				Preface:       ch.Template.Preface,
				Postscript:    ch.Template.Postscript,
				BaseURL:       baseURL,
				Language:      ch.Language,
				Summarizer:    summarizer,
				TitleTemplate: ch.Template.Title,
			})
		}

		ws := []worker.Worker{}
		if collector != nil {
			slog.Info("starting V2EX collector for nodes", "nodes", collector.Nodes)
			ws = append(ws, collector)
		}
		if hnCollector != nil {
			slog.Info("starting Hacker News collector for lists", "lists", hnCollector.Lists)
			ws = append(ws, hnCollector)
		}
		ws = append(ws, builders...)
		mgr := worker.NewManager(ws...)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Signal handling for systemd
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			s := <-sigc
			log.Printf("received signal: %s, shutting down", s)
			cancel()
		}()

		if err := mgr.Start(ctx); err != nil {
			return err
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
