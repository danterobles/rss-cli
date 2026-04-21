package config

import (
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	DataDir      string
	DatabasePath string
}

func Load() (Config, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Config{}, fmt.Errorf("resolve home: %w", err)
		}
		base = filepath.Join(home, ".config")
	}

	dataDir := filepath.Join(base, "rss-cli")
	return Config{
		DataDir:      dataDir,
		DatabasePath: filepath.Join(dataDir, "data.db"),
	}, nil
}
