package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/charliewilco/friday/internal/config"
	"github.com/charliewilco/friday/internal/ollama"
	"github.com/charliewilco/friday/internal/store"
)

func newInitCommand() *cobra.Command {
	var assumeYes bool
	var nameOverride string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Bootstrap a Friday project in the current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			configPath := filepath.Join(cwd, config.FileName)
			if _, err := os.Stat(configPath); err == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "%s already exists\n", configPath)
				return nil
			}

			ctx, cancel := signalContext()
			defer cancel()

			defaultName := kebabCase(filepath.Base(cwd))
			detected, err := detectContentPaths(cwd)
			if err != nil {
				return err
			}
			if nameOverride != "" {
				defaultName = kebabCase(nameOverride)
			}

			cfg := config.Default()
			cfg.Name = defaultName
			cfg.Content.Paths = detected
			if host := strings.TrimSpace(os.Getenv("FRIDAY_OLLAMA_HOST")); host != "" {
				cfg.Ollama.Host = host
			}

			if !assumeYes {
				if err := promptInitConfig(cmd, &cfg); err != nil {
					return err
				}
			}

			if cfg.Name == "" {
				return fmt.Errorf("init failed: project name could not be inferred. Re-run with --name <name>")
			}
			if len(cfg.Content.Paths) == 0 {
				return fmt.Errorf("init failed: no content paths detected. Re-run with --name and edit %s manually", config.FileName)
			}

			client := ollama.New(cfg.Ollama.Host)
			if err := client.Ping(ctx); err != nil {
				return err
			}
			if err := ensureModel(ctx, client, cfg.Models.Chat, "chat"); err != nil {
				return err
			}
			if err := ensureModel(ctx, client, cfg.Models.Embeddings, "embedding"); err != nil {
				return err
			}

			if err := writeConfig(configPath, cfg); err != nil {
				return err
			}

			loaded, err := config.Load(configPath)
			if err != nil {
				return err
			}
			stateDir, err := loaded.StateDir()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(stateDir, 0o755); err != nil {
				return err
			}
			dbPath, err := loaded.DatabasePath()
			if err != nil {
				return err
			}
			db, err := store.Open(dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			fmt.Fprintf(cmd.OutOrStdout(), "created %s\n", configPath)
			fmt.Fprintln(cmd.OutOrStdout(), "Run 'friday embed' to index your corpus.")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "skip prompts and use defaults")
	cmd.Flags().StringVar(&nameOverride, "name", "", "override project name")
	return cmd
}

func detectContentPaths(root string) ([]string, error) {
	candidates := []string{"src/content", "content", "posts", "notes"}
	var found []string
	for _, candidate := range candidates {
		info, err := os.Stat(filepath.Join(root, candidate))
		if err == nil && info.IsDir() {
			found = append(found, filepath.ToSlash(candidate))
		}
	}

	type counter struct {
		path  string
		count int
	}
	var counts []counter
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." {
			return nil
		}
		if depth := strings.Count(filepath.ToSlash(rel), "/") + 1; depth > 3 {
			return filepath.SkipDir
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil
		}
		count := 0
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if ext == ".md" || ext == ".mdx" {
				count++
			}
		}
		if count > 10 {
			counts = append(counts, counter{path: filepath.ToSlash(rel), count: count})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	for _, count := range counts {
		if !slices.Contains(found, count.path) {
			found = append(found, count.path)
		}
	}
	slices.Sort(found)
	return found, nil
}

func promptInitConfig(cmd *cobra.Command, cfg *config.Config) error {
	reader := bufio.NewReader(cmd.InOrStdin())
	questions := []struct {
		label string
		value *string
	}{
		{label: fmt.Sprintf("Project name [%s]: ", cfg.Name), value: &cfg.Name},
		{label: fmt.Sprintf("Content paths (comma-separated) [%s]: ", strings.Join(cfg.Content.Paths, ", ")), value: nil},
		{label: fmt.Sprintf("Chat model [%s]: ", cfg.Models.Chat), value: &cfg.Models.Chat},
		{label: fmt.Sprintf("Embedding model [%s]: ", cfg.Models.Embeddings), value: &cfg.Models.Embeddings},
	}

	for idx, question := range questions {
		fmt.Fprint(cmd.OutOrStdout(), question.label)
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if idx == 1 {
			cfg.Content.Paths = splitCSV(line)
			continue
		}
		*question.value = line
	}
	cfg.Name = kebabCase(cfg.Name)
	return nil
}

func splitCSV(input string) []string {
	parts := strings.Split(input, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, filepath.ToSlash(part))
		}
	}
	return result
}

func writeConfig(path string, cfg config.Config) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = fmt.Fprintf(file, "name = %q\n\n[models]\nchat = %q\nembeddings = %q\n\n[content]\npaths = [%s]\nextensions = [\".md\", \".mdx\"]\nignore = []\nmax_chunk_tokens = 800\nmin_chunk_tokens = 50\ninclude_frontmatter = true\n\n[retrieval]\ntop_k = 6\nsimilarity_threshold = 0.0\n\n[ollama]\nhost = %q\n",
		cfg.Name,
		cfg.Models.Chat,
		cfg.Models.Embeddings,
		joinQuoted(cfg.Content.Paths),
		cfg.Ollama.Host,
	)
	return err
}

func joinQuoted(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, fmt.Sprintf("%q", value))
	}
	return strings.Join(quoted, ", ")
}
