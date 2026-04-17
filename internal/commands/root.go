package commands

import "github.com/spf13/cobra"

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "friday",
		Short:        "Chat with your markdown corpus through Ollama",
		SilenceUsage: true,
	}

	cmd.AddCommand(
		newInitCommand(),
		newEmbedCommand(),
		newRunCommand(),
		newAskCommand(),
		newResetCommand(),
	)
	return cmd
}
