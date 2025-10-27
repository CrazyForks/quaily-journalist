package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"quaily-journalist/internal/quaily"

	"github.com/spf13/cobra"
)

var publishCmd = &cobra.Command{
	Use:   "publish <markdown_path> <channel_slug>",
	Short: "Publish a markdown file to Quaily",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("requires <markdown_path> and <channel_slug>")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := GetConfig()
		if cfg.Quaily.BaseURL == "" || cfg.Quaily.APIKey == "" {
			return fmt.Errorf("quaily config missing: set quaily.base_url and quaily.api_key in config.yaml")
		}
		tm := 20 * time.Second
		cli := quaily.New(cfg.Quaily.BaseURL, cfg.Quaily.APIKey, tm)
		ctx, cancel := context.WithTimeout(context.Background(), tm)
		defer cancel()
		mdPath := args[0]
		channelSlug := args[1]
		if err := quaily.PublishMarkdownFile(ctx, cli, mdPath, channelSlug); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Published %s to Quaily channel %s\n", mdPath, channelSlug)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(publishCmd)
}
