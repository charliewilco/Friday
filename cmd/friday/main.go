package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/charliewilco/friday/internal/commands"
)

func main() {
	if err := commands.NewRootCommand().Execute(); err != nil {
		var exitErr interface{ ExitCode() int }
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}

		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
