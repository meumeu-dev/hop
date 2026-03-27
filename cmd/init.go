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
		fmt.Println("Configurer Cloudflare ? (pour les tunnels)")
		fmt.Println("  1) Non, plus tard")
		fmt.Println("  2) Oui, interactif")
		fmt.Println("  3) Importer un fichier .env (local ou URL)")
		fmt.Print("Choix [1]: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		if choice == "2" || choice == "o" || choice == "oui" {
			fmt.Println()
			configCFCmd.Run(cmd, nil)
		} else if choice == "3" {
			fmt.Print("Chemin ou URL du fichier .env: ")
			envPath, _ := reader.ReadString('\n')
			envPath = strings.TrimSpace(envPath)
			if envPath != "" {
				cfEnvFile = envPath
				fmt.Println()
				configCFCmd.Run(cmd, nil)
				cfEnvFile = "" // reset
			}
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
