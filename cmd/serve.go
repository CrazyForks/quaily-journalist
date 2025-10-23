package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"quaily-journalist/internal/ai"
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

		// V2EX collector setup with union of nodes across channels using v2ex
		v2c := v2ex.NewClient(cfg.Sources.V2EX.BaseURL, cfg.Sources.V2EX.Token)
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
		collector := &worker.V2EXCollector{
			Client:   v2c,
			Store:    store,
			Nodes:    nodes,
			Interval: interval,
		}

		// Prepare AI summarizer (optional)
		var summarizer ai.Summarizer
		if cfg.OpenAI.APIKey != "" {
			summarizer = ai.NewOpenAI(ai.Config{APIKey: cfg.OpenAI.APIKey, Model: cfg.OpenAI.Model, BaseURL: cfg.OpenAI.BaseURL})
		}

		// Newsletter builders (one per channel)
		var builders []worker.Worker
		for _, ch := range cfg.Newsletters.Channels {
			sd, err := time.ParseDuration(ch.ItemSkipDuration)
			if err != nil {
				return fmt.Errorf("invalid item_skip_duration for channel %s: %w", ch.Name, err)
			}
			builders = append(builders, &worker.NewsletterBuilder{
				Store:        store,
				Source:       strings.ToLower(ch.Source),
				Channel:      ch.Name,
				Frequency:    strings.ToLower(ch.Frequency),
				TopN:         ch.TopN,
				MinItems:     ch.MinItems,
				OutputDir:    ch.OutputDir,
				Interval:     30 * time.Minute,
				Nodes:        ch.Nodes,
				SkipDuration: sd,
				Preface:      ch.Preface,
				Postscript:   ch.Postscript,
				BaseURL:      cfg.Sources.V2EX.BaseURL,
				Language:     ch.Language,
				Summarizer:   summarizer,
			})
		}

		ws := []worker.Worker{collector}
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
