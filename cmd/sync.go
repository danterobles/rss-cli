package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(syncCmd)
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync all configured feeds",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWithDependencies(cmd, func(ctx context.Context, deps *dependencies) error {
			count, err := deps.Service.SyncAll(ctx)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "synced %d new article(s)\n", count)
			return nil
		})
	},
}
