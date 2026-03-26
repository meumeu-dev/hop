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
		reader := bufio.NewReader(os.Stdin)

		fmt.Println("=== hop init ===")
		fmt.Println()

		if err := config.Init(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		// Ask for Cloudflare config
		fmt.Print("Domaine Cloudflare (laisser vide pour skip): ")
		domain, _ := reader.ReadString('\n')
		domain = strings.TrimSpace(domain)

		cfg, _ := config.Load()

		if domain != "" {
			fmt.Print("Chemin vers le fichier token CF (ex: ~/token-cf.env): ")
			envFile, _ := reader.ReadString('\n')
			envFile = strings.TrimSpace(envFile)

			cfg.Cloudflare = config.CloudflareConfig{
				Domain:  domain,
				EnvFile: envFile,
			}
		}

		cfg.Save()

		fmt.Println()
		fmt.Println("→ hop est pret !")
		fmt.Println()
		fmt.Println("Prochaines etapes:")
		fmt.Println("  hop pair                — pairer avec une autre machine")
		fmt.Println("  hop add machine pc1 192.168.0.10 --user user")
		fmt.Println("  hop ssh pc1             — se connecter")
		fmt.Println("  hop dashboard           — interface web")
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
