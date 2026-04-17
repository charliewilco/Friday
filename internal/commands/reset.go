package commands

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newResetCommand() *cobra.Command {
	var assumeYes bool

	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Delete Friday state for the current project",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			stateDir, err := cfg.StateDir()
			if err != nil {
				return err
			}

			if !assumeYes {
				fmt.Fprintf(cmd.OutOrStdout(), "Delete %s? [y/N]: ", stateDir)
				line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				if err != nil {
					return err
				}
				switch strings.ToLower(strings.TrimSpace(line)) {
				case "y", "yes":
				default:
					fmt.Fprintln(cmd.OutOrStdout(), "aborted")
					return nil
				}
			}

			if err := os.RemoveAll(stateDir); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", stateDir)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "skip confirmation")
	return cmd
}
