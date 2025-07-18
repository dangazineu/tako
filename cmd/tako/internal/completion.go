package internal

import (
	"github.com/spf13/cobra"
)

func NewCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate completion script",
		Long: `To load completions:

Bash:

  $ source <(tako completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ tako completion bash > /etc/bash_completion.d/tako
  # macOS:
  $ tako completion bash > /usr/local/etc/bash_completion.d/tako

Zsh:

  # If shell completion is not already enabled in your environment,
  # you will need to enable it.  You can execute the following once:

  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ tako completion zsh > "${fpath[1]}/_tako"

  # You will need to start a new shell for this setup to take effect.

Fish:

  $ tako completion fish | source

  # To load completions for each session, execute once:
  $ tako completion fish > ~/.config/fish/completions/tako.fish

PowerShell:

  PS> tako completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> tako completion powershell > tako.ps1
  # and source this file from your PowerShell profile.
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(cmd.OutOrStdout())
			case "zsh":
				return cmd.Root().GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return cmd.Root().GenFishCompletion(cmd.OutOrStdout(), true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
			}
			return nil
		},
	}
	return cmd
}
