package commands

import (
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/charliewilco/friday/internal/ollama"
	"github.com/charliewilco/friday/internal/repl"
	"github.com/charliewilco/friday/internal/retrieve"
)

func newAskCommand() *cobra.Command {
	var model string
	var topK int

	cmd := &cobra.Command{
		Use:   "ask [question]",
		Short: "Ask Friday a single question",
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

			if model == "" {
				model = cfg.Config.Models.Chat
			}
			if topK == 0 {
				topK = cfg.Config.Retrieval.TopK
			}

			query := strings.TrimSpace(strings.Join(args, " "))
			if query == "" {
				input, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return err
				}
				query = strings.TrimSpace(string(input))
			}

			return repl.Ask(ctx, &repl.Shell{
				Stdout: cmd.OutOrStdout(),
				Pipeline: retrieve.Pipeline{
					Config: cfg,
					Store:  db,
					Client: client,
				},
				Store: db,
				Model: model,
				TopK:  topK,
			}, query)
		},
	}

	cmd.Flags().StringVar(&model, "model", "", "override the chat model for this request")
	cmd.Flags().IntVar(&topK, "top-k", 0, "override top-k retrieval for this request")
	return cmd
}
