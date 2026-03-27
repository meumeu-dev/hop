package cmd

import (
	"fmt"
	"os"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var exitCmd = &cobra.Command{
	Use:   "exit",
	Short: "Supprime toute trace de hop (mode sandbox)",
	Long: `Supprime le dossier de config temporaire. Zero trace sur le systeme.
En mode installe (hop install), utilise hop uninstall a la place.`,
	Run: func(cmd *cobra.Command, args []string) {
		if config.IsInstalled() {
			fmt.Println("hop est en mode installe.")
			fmt.Println("Utilise 'hop uninstall' pour tout supprimer.")
			os.Exit(1)
		}

		hopDir := config.HopDir()
		if _, err := os.Stat(hopDir); os.IsNotExist(err) {
			fmt.Println("Rien a nettoyer.")
			return
		}

		if err := os.RemoveAll(hopDir); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("→ hop nettoye. Zero trace.")
	},
}

func init() {
	rootCmd.AddCommand(exitCmd)
}
