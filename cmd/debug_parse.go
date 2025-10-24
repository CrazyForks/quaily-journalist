package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"quaily-journalist/internal/markdown"
)

var debugParseCmd = &cobra.Command{
	Use:   "debug-parse <markdown_path>",
	Short: "Debug: parse a markdown and print frontmatter keys",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]
		doc, err := markdown.ParseFile(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "frontmatter keys: ")
		first := true
		for k := range doc.Frontmatter {
			if !first {
				fmt.Fprint(os.Stdout, ", ")
			}
			fmt.Fprint(os.Stdout, k)
			first = false
		}
		fmt.Fprintln(os.Stdout)
		fmt.Fprintf(os.Stdout, "body bytes: %d\n", len(doc.Body))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(debugParseCmd)
}
