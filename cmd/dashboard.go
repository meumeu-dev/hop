package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/meumeu-dev/hop/internal/dashboard"
	"github.com/spf13/cobra"
)

var dashPort int
var dashNoOpen bool
var dashBind string

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Lance le dashboard web",
	Run: func(cmd *cobra.Command, args []string) {
		dashboard.DashboardVersion = version

		// If bind not specified, ask
		if dashBind == "" {
			reader := bufio.NewReader(os.Stdin)
			fmt.Println("Ecoute du dashboard:")
			fmt.Println("  1) Localhost uniquement (127.0.0.1)")
			fmt.Println("  2) Reseau local (0.0.0.0) — protege par mot de passe")
			fmt.Print("Choix [1]: ")
			choice, _ := reader.ReadString('\n')
			choice = strings.TrimSpace(choice)
			if choice == "2" {
				dashBind = "0.0.0.0"
			} else {
				dashBind = "127.0.0.1"
			}
			fmt.Println()
		}

		// If network mode, require a password
		dashPassword := ""
		if dashBind == "0.0.0.0" {
			reader := bufio.NewReader(os.Stdin)
			fmt.Print("Mot de passe pour le dashboard: ")
			pw, _ := reader.ReadString('\n')
			dashPassword = strings.TrimSpace(pw)
			if len(dashPassword) < 4 {
				fmt.Fprintln(os.Stderr, "Mot de passe requis pour l'acces reseau (min 4 caracteres)")
				os.Exit(1)
			}
			fmt.Println()
		}

		fmt.Println("→ Démarrage du dashboard...")
		if err := dashboard.StartWithBind(dashPort, dashBind, dashPassword, !dashNoOpen); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	dashboardCmd.Flags().IntVar(&dashPort, "port", 8080, "Port du dashboard")
	dashboardCmd.Flags().StringVar(&dashBind, "bind", "", "Adresse d'ecoute (127.0.0.1 ou 0.0.0.0)")
	dashboardCmd.Flags().BoolVar(&dashNoOpen, "no-open", false, "Ne pas ouvrir le navigateur")
	rootCmd.AddCommand(dashboardCmd)
}
