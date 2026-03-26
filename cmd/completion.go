package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish]",
	Short: "Genere le script d'autocompletion",
	Long: `Genere le script d'autocompletion pour le shell specifie.

Bash:
  source <(hop completion bash)
  # Ou pour rendre permanent:
  hop completion bash > /etc/bash_completion.d/hop

Zsh:
  source <(hop completion zsh)
  # Ou pour rendre permanent:
  hop completion zsh > "${fpath[1]}/_hop"

Fish:
  hop completion fish | source
  # Ou pour rendre permanent:
  hop completion fish > ~/.config/fish/completions/hop.fish
`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"bash", "zsh", "fish"},
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			rootCmd.GenFishCompletion(os.Stdout, true)
		}
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
