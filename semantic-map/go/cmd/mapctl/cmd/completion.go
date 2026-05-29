package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// newCompletionCmd returns cobra's standard `completion` subcommand,
// generating shell completions for bash/zsh/fish/powershell. The
// implementation is delegated entirely to cobra so we get the maintained
// generators for free.
func newCompletionCmd() *cobra.Command {
	completionCmd := &cobra.Command{
		Use:                   "completion [bash|zsh|fish|powershell]",
		Short:                 "Generate shell completion script",
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		Long: "Generate the autocompletion script for mapctl in the chosen shell.\n" +
			"Examples:\n  mapctl completion bash > /etc/bash_completion.d/mapctl\n" +
			"  mapctl completion zsh  > \"${fpath[1]}/_mapctl\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			}
			return nil
		},
	}
	return completionCmd
}
