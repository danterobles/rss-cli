package cmd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/danterobles/rss-cli/internal/config"
	"github.com/danterobles/rss-cli/internal/rss"
	"github.com/danterobles/rss-cli/internal/storage"
	"github.com/danterobles/rss-cli/internal/tui"
	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

var rootCmd = &cobra.Command{
	Use:   "rss-cli",
	Short: "RSS reader for the terminal",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		deps, err := newDependencies(ctx)
		if err != nil {
			return err
		}
		defer deps.Close()

		model, err := tui.NewModel(ctx, deps.Repository, deps.Service)
		if err != nil {
			return err
		}

		program := tea.NewProgram(model, tea.WithAltScreen())
		_, err = program.Run()
		return err
	},
}

type dependencies struct {
	DB         *sql.DB
	Repository *storage.Repository
	Service    *rss.Service
}

func (d *dependencies) Close() error {
	if d == nil || d.DB == nil {
		return nil
	}
	return d.DB.Close()
}

func Execute() error {
	ctx := context.Background()
	return rootCmd.ExecuteContext(ctx)
}

func newDependencies(ctx context.Context) (*dependencies, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	db, err := sql.Open("sqlite", cfg.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	repo := storage.NewRepository(db)
	if err := repo.Init(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	service := rss.NewService(repo)
	return &dependencies{
		DB:         db,
		Repository: repo,
		Service:    service,
	}, nil
}

func runWithDependencies(cmd *cobra.Command, fn func(context.Context, *dependencies) error) error {
	deps, err := newDependencies(cmd.Context())
	if err != nil {
		return err
	}
	defer deps.Close()

	if err := fn(cmd.Context(), deps); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("not found: %w", err)
		}
		return err
	}
	return nil
}
