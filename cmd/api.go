package cmd

import (
	"fmt"
	"os"

	"github.com/meumeu-dev/hop/internal/dashboard"
	"github.com/spf13/cobra"
)

var apiPort int
var apiDisable bool

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Active/désactive l'API pour connexion depuis un autre hop",
	Run: func(cmd *cobra.Command, args []string) {
		if apiDisable {
			fmt.Println("→ API désactivée.")
			// TODO: kill running API process / systemd service
			return
		}

		fmt.Printf("→ API activée sur le port %d\n", apiPort)
		fmt.Println("  Accessible via Cloudflare Tunnel ou réseau local.")
		fmt.Println("  Ctrl+C pour arrêter.")
		if err := dashboard.StartAPI(apiPort); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	apiCmd.Flags().IntVar(&apiPort, "port", 9090, "Port de l'API")
	apiCmd.Flags().BoolVar(&apiDisable, "disable", false, "Désactive l'API")
	rootCmd.AddCommand(apiCmd)
}
