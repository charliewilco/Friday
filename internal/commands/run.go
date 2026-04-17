package commands

import (
	"github.com/spf13/cobra"

	"github.com/charliewilco/friday/internal/ollama"
	"github.com/charliewilco/friday/internal/repl"
	"github.com/charliewilco/friday/internal/retrieve"
)

func newRunCommand() *cobra.Command {
	var model string
	var topK int

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start the Friday REPL",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			ctx, cancel := signalContext()
			defer cancel()

			client := ollama.New(cfg.Config.Ollama.Host)
			if err := client.Ping(ctx); err != nil {
				return err
			}
			if err := ensureModel(ctx, client, cfg.Config.Models.Chat, "chat"); err != nil {
				return err
			}
			if err := ensureModel(ctx, client, cfg.Config.Models.Embeddings, "embedding"); err != nil {
				return err
			}

			db, _, err := openStoreForConfig(cfg)
			if err != nil {
				return err
			}
			defer db.Close()
			if dim, ok, err := db.EmbeddingDim(ctx); err != nil {
				return err
			} else if ok {
				if err := db.EnsureVectorTable(ctx, dim); err != nil {
					return err
				}
			}

			if model == "" {
				model = cfg.Config.Models.Chat
			}
			if topK == 0 {
				topK = cfg.Config.Retrieval.TopK
			}

			return repl.Run(ctx, &repl.Shell{
				Stdout: cmd.OutOrStdout(),
				Stdin:  cmd.InOrStdin(),
				Pipeline: retrieve.Pipeline{
					Config: cfg,
					Store:  db,
					Client: client,
				},
				Store: db,
				Model: model,
				TopK:  topK,
			})
		},
	}

	cmd.Flags().StringVar(&model, "model", "", "override the chat model for this session")
	cmd.Flags().IntVar(&topK, "top-k", 0, "override top-k retrieval for this session")
	return cmd
}
