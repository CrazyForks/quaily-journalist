package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"quaily-journalist/internal/quaily"

	"github.com/spf13/cobra"
)

var sendCmd = &cobra.Command{
	Use:   "send <path_or_slug> <channel_slug>",
	Short: "Deliver a Quaily post by slug or markdown file",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("requires <path_or_slug> and <channel_slug>")
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

		pathOrSlug := args[0]
		channelSlug := args[1]
		if err := quaily.DeliverMarkdownOrSlug(ctx, cli, pathOrSlug, channelSlug); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Delivered post '%s' on channel %s\n", pathOrSlug, channelSlug)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(sendCmd)
}
