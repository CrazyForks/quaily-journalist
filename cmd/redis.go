package cmd

import "github.com/spf13/cobra"

// redisCmd groups Redis-related subcommands.
var redisCmd = &cobra.Command{
	Use:   "redis",
	Short: "Redis utilities",
}

func init() {
	rootCmd.AddCommand(redisCmd)
}
