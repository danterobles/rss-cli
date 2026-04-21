package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(removeCmd)
}

var removeCmd = &cobra.Command{
	Use:   "remove <url>",
	Short: "Remove a feed by URL",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithDependencies(cmd, func(ctx context.Context, deps *dependencies) error {
			if err := deps.Repository.DeleteFeedByURL(ctx, args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", args[0])
			return nil
		})
	},
}
