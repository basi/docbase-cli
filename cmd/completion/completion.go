package completion

import (
	"os"

	"github.com/basi/docbase-cli/cmd/root"
	"github.com/spf13/cobra"
)

var (
	// CompletionCmd represents the completion command
	CompletionCmd = &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate completion script",
		Long: `Generate shell completion script for DocBase CLI.

To load completions:

Bash:
  $ source <(docbase completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ docbase completion bash > /etc/bash_completion.d/docbase
  # macOS:
  $ docbase completion bash > /usr/local/etc/bash_completion.d/docbase

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it.  You can execute the following once:

  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ docbase completion zsh > "${fpath[1]}/_docbase"

  # You will need to start a new shell for this setup to take effect.

Fish:
  $ docbase completion fish | source

  # To load completions for each session, execute once:
  $ docbase completion fish > ~/.config/fish/completions/docbase.fish

PowerShell:
  PS> docbase completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> docbase completion powershell > docbase.ps1
  # and source this file from your PowerShell profile.
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.ExactValidArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			switch args[0] {
			case "bash":
				cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			}
		},
	}
)

func init() {
	// Add completion command to root command
	root.AddCommand(CompletionCmd)
}