package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(addCmd)
}

var addCmd = &cobra.Command{
	Use:   "add <url>",
	Short: "Add a new RSS/Atom feed",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithDependencies(cmd, func(ctx context.Context, deps *dependencies) error {
			feed, count, err := deps.Service.AddFeed(ctx, args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added %q with %d article(s)\n", feed.Title, count)
			return nil
		})
	},
}
