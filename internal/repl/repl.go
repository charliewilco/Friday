package repl

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/charliewilco/friday/internal/retrieve"
	"github.com/charliewilco/friday/internal/store"
)

type Shell struct {
	Stdout   io.Writer
	Stdin    io.Reader
	Pipeline retrieve.Pipeline
	Store    *store.Store
	Model    string
	TopK     int
}

func Run(ctx context.Context, shell *Shell) error {
	files, chunks, err := shell.Store.CountStats(ctx)
	if err != nil {
		return err
	}
	if chunks == 0 {
		return fmt.Errorf("Friday has no indexed chunks yet. Run 'friday embed' first")
	}

	fmt.Fprintf(shell.Stdout, "Friday ready. %d chunks across %d files. Ctrl+D to exit.\n", chunks, files)

	scanner := bufio.NewScanner(shell.Stdin)
	prompt := lipgloss.NewStyle().Foreground(lipgloss.Color("45")).Render("> ")
	for {
		fmt.Fprint(shell.Stdout, prompt)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			return nil
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, ":") {
			handled, exit, err := handleColonCommand(ctx, line, shell)
			if err != nil {
				return err
			}
			if handled && exit {
				return nil
			}
			if handled {
				continue
			}
		}

		if err := askOnce(ctx, shell, line); err != nil {
			return err
		}
	}
}

func Ask(ctx context.Context, shell *Shell, query string) error {
	return askOnce(ctx, shell, strings.TrimSpace(query))
}

func askOnce(ctx context.Context, shell *Shell, query string) error {
	result, err := shell.Pipeline.Ask(ctx, shell.Model, query, shell.TopK, func(token string) {
		fmt.Fprint(shell.Stdout, token)
	})
	if err != nil {
		return err
	}

	if len(result.Sources) == 0 {
		_, err := fmt.Fprintln(shell.Stdout, "No relevant excerpts found in the indexed corpus.")
		return err
	}

	if _, err := fmt.Fprintln(shell.Stdout); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(shell.Stdout, "\nSources:"); err != nil {
		return err
	}

	seen := map[string]struct{}{}
	for _, source := range result.Sources {
		key := source.Path + "||" + source.HeadingPath
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if _, err := fmt.Fprintf(shell.Stdout, "  %s — %s\n", source.Path, source.HeadingPath); err != nil {
			return err
		}
	}
	return nil
}
