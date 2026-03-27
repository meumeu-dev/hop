package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Configure hop depuis zero",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("=== hop init ===")
		if config.IsInstalled() {
			fmt.Println("Mode: installe (~/.hop/)")
		} else {
			fmt.Printf("Mode: sandbox (%s) — disparait au reboot\n", config.HopDir())
			fmt.Println("  hop install pour rendre permanent")
		}
		fmt.Println()

		if err := config.Init(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Configurer Cloudflare maintenant ? (pour les tunnels) [o/N]: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(strings.ToLower(choice))

		if choice == "o" || choice == "oui" || choice == "y" || choice == "yes" {
			fmt.Println()
			configCFCmd.Run(cmd, nil)
		} else {
			cfg, _ := config.Load()
			if cfg != nil {
				cfg.Save()
			}
			fmt.Println()
			fmt.Println("→ hop est pret !")
			fmt.Println()
			fmt.Println("Prochaines etapes:")
			fmt.Println("  hop pair               — pairer avec une autre machine")
			fmt.Println("  hop config cf          — configurer Cloudflare plus tard")
			fmt.Println("  hop ssh <machine>      — se connecter")
			fmt.Println("  hop dashboard          — interface web")
		}
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
