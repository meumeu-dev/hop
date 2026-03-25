package cmd

import (
	"fmt"
	"os"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/meumeu-dev/hop/internal/dashboard"
	"github.com/spf13/cobra"
)

var apiPort int
var apiReadOnly bool
var apiResetKey bool
var apiShowKey bool

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Active l'API pour connexion depuis un autre hop",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if apiResetKey {
			cfg.API.Key = config.GenerateAPIKey()
			cfg.Save()
			fmt.Printf("→ Nouvelle clé API: %s\n", cfg.API.Key)
			return
		}

		if apiShowKey {
			if cfg.API.Key == "" {
				fmt.Println("Aucune clé API configurée. Lance 'hop api' pour en générer une.")
			} else {
				fmt.Println(cfg.API.Key)
			}
			return
		}

		cfg.API.ReadOnly = apiReadOnly
		cfg.Save()

		if err := dashboard.StartAPI(apiPort); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	apiCmd.Flags().IntVar(&apiPort, "port", 9090, "Port de l'API")
	apiCmd.Flags().BoolVar(&apiReadOnly, "read-only", false, "Mode lecture seule")
	apiCmd.Flags().BoolVar(&apiResetKey, "reset-key", false, "Génère une nouvelle clé API")
	apiCmd.Flags().BoolVar(&apiShowKey, "show-key", false, "Affiche la clé API actuelle")
	rootCmd.AddCommand(apiCmd)
}
