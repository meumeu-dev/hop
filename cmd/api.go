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
		secrets, _ := config.LoadSecrets()

		if apiResetKey {
			secrets.APIKey = config.GenerateAPIKey()
			secrets.Save()
			fmt.Printf("-> Nouvelle cle API: %s\n", secrets.APIKey)
			return
		}

		if apiShowKey {
			if secrets.APIKey == "" {
				fmt.Println("Aucune cle API configuree. Lance 'hop api' pour en generer une.")
			} else {
				fmt.Println(secrets.APIKey)
			}
			return
		}

		dashboard.DashboardVersion = version
		if err := dashboard.StartAPI(apiPort, apiReadOnly); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	apiCmd.Flags().IntVar(&apiPort, "port", 9090, "Port de l'API")
	apiCmd.Flags().BoolVar(&apiReadOnly, "read-only", false, "Mode lecture seule")
	apiCmd.Flags().BoolVar(&apiResetKey, "reset-key", false, "Genere une nouvelle cle API")
	apiCmd.Flags().BoolVar(&apiShowKey, "show-key", false, "Affiche la cle API actuelle")
	rootCmd.AddCommand(apiCmd)
}
