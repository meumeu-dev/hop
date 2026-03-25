package cmd

import (
	"fmt"
	"os"

	"github.com/meumeu-dev/hop/internal/dashboard"
	"github.com/spf13/cobra"
)

var dashPort int
var dashNoOpen bool

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Lance le dashboard web",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("→ Démarrage du dashboard...")
		if err := dashboard.Start(dashPort, !dashNoOpen); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	dashboardCmd.Flags().IntVar(&dashPort, "port", 8080, "Port du dashboard")
	dashboardCmd.Flags().BoolVar(&dashNoOpen, "no-open", false, "Ne pas ouvrir le navigateur")
	rootCmd.AddCommand(dashboardCmd)
}
