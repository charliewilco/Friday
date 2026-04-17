package repl

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/charliewilco/friday/internal/store"
)

func handleColonCommand(ctx context.Context, line string, shell *Shell) (handled bool, shouldExit bool, err error) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return false, false, nil
	}

	switch fields[0] {
	case ":help":
		_, err = fmt.Fprintln(shell.Stdout, ":help, :quit, :q, :stats, :sources <query>, :k <n>")
		return true, false, err
	case ":quit", ":q":
		return true, true, nil
	case ":stats":
		files, chunks, err := shell.Store.CountStats(ctx)
		if err != nil {
			return true, false, err
		}
		lastEmbed, ok, err := shell.Store.LastEmbedAt(ctx)
		if err != nil {
			return true, false, err
		}
		if ok {
			_, err = fmt.Fprintf(shell.Stdout, "%d files, %d chunks, last embed %s\n", files, chunks, lastEmbed.Format("2006-01-02 15:04:05"))
			return true, false, err
		}
		_, err = fmt.Fprintf(shell.Stdout, "%d files, %d chunks, never embedded\n", files, chunks)
		return true, false, err
	case ":sources":
		if len(fields) < 2 {
			_, err = fmt.Fprintln(shell.Stdout, "usage: :sources <query>")
			return true, false, err
		}
		query := strings.TrimSpace(strings.TrimPrefix(line, ":sources"))
		sources, err := shell.Pipeline.Sources(ctx, query, shell.TopK)
		if err != nil {
			return true, false, err
		}
		err = printSources(shell.Stdout, sources)
		return true, false, err
	case ":k":
		if len(fields) != 2 {
			_, err = fmt.Fprintln(shell.Stdout, "usage: :k <n>")
			return true, false, err
		}
		value, err := strconv.Atoi(fields[1])
		if err != nil || value <= 0 {
			return true, false, fmt.Errorf("invalid k value %q", fields[1])
		}
		shell.TopK = value
		_, err = fmt.Fprintf(shell.Stdout, "top-k set to %d\n", value)
		return true, false, err
	default:
		return false, false, nil
	}
}

func printSources(out io.Writer, sources []store.SearchResult) error {
	for _, source := range sources {
		if _, err := fmt.Fprintf(out, "%s — %s\n", source.Path, source.HeadingPath); err != nil {
			return err
		}
	}
	return nil
}
