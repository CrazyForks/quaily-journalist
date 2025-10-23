package cmd

import (
	"context"
	"fmt"
	"time"

	"quaily-journalist/internal/redisclient"

	"github.com/spf13/cobra"
)

// pingCmd pings the configured Redis server.
var pingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Ping Redis and print PONG",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := GetConfig()

		rdb := redisclient.New(cfg.Redis)
		defer rdb.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		res, err := rdb.Ping(ctx).Result()
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), res)
		return nil
	},
}

func init() {
	redisCmd.AddCommand(pingCmd)
}
