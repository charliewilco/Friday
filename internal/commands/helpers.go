package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/charliewilco/friday/internal/config"
	"github.com/charliewilco/friday/internal/ollama"
	"github.com/charliewilco/friday/internal/store"
)

func loadConfig() (config.Loaded, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return config.Loaded{}, err
	}
	cfg, err := config.Load(filepath.Join(cwd, config.FileName))
	if err != nil {
		return config.Loaded{}, err
	}
	for _, warning := range cfg.Warnings {
		fmt.Fprintln(os.Stderr, warning)
	}
	return cfg, nil
}

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func ensureModel(ctx context.Context, client *ollama.Client, model, label string) error {
	ok, err := client.ModelExists(ctx, model)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s model %q is not pulled locally. Pull it with: ollama pull %s", label, model, model)
	}
	return nil
}

func openStoreForConfig(cfg config.Loaded) (*store.Store, string, error) {
	dbPath, err := cfg.DatabasePath()
	if err != nil {
		return nil, "", err
	}
	if _, err := os.Stat(dbPath); err != nil {
		return nil, "", fmt.Errorf("Friday state not found at %s. Run 'friday init' first", dbPath)
	}
	db, err := store.Open(dbPath)
	if err != nil {
		return nil, "", err
	}
	return db, dbPath, nil
}

func kebabCase(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	var builder strings.Builder
	lastDash := false
	for _, r := range input {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			builder.WriteRune(r)
			lastDash = false
		case !lastDash:
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}
