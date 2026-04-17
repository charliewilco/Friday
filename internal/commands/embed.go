package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/charliewilco/friday/internal/embed"
	"github.com/charliewilco/friday/internal/ollama"
)

func newEmbedCommand() *cobra.Command {
	var force bool
	var dryRun bool
	var verbose bool

	cmd := &cobra.Command{
		Use:   "embed",
		Short: "Index the configured markdown corpus into Friday's SQLite store",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			ctx, cancel := signalContext()
			defer cancel()

			db, _, err := openStoreForConfig(cfg)
			if err != nil {
				return err
			}
			defer db.Close()

			client := ollama.New(cfg.Config.Ollama.Host)
			if err := client.Ping(ctx); err != nil {
				return err
			}
			if err := ensureModel(ctx, client, cfg.Config.Models.Embeddings, "embedding"); err != nil {
				return err
			}

			result, err := embed.Run(ctx, cfg, db, client, embed.Options{
				Force:   force,
				DryRun:  dryRun,
				Verbose: verbose,
				Stderr:  os.Stderr,
			})
			if err != nil {
				if strings.Contains(err.Error(), "embedding dimension mismatch") {
					return ExitError{Code: 2, Err: err}
				}
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "embed complete: %d files (%d indexed, %d unchanged, %d removed), %d chunks total, took %s\n",
				result.TotalFiles, result.IndexedFiles, result.UnchangedFiles, result.RemovedFiles, result.TotalChunks, result.Duration.Round(100000000))
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "re-embed every file regardless of hash")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report actions without writing changes")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "log every file, not just changed ones")
	return cmd
}
