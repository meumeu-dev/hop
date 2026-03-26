package cmd

import (
	"fmt"
	"os"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Remet la config a zero (garde le binaire)",
	Run: func(cmd *cobra.Command, args []string) {
		hopDir := config.HopDir()

		if _, err := os.Stat(hopDir); os.IsNotExist(err) {
			fmt.Println("Rien a reset, ~/.hop n'existe pas.")
			return
		}

		if err := os.RemoveAll(hopDir); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if err := config.Init(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("→ Config remise a zero.")
		fmt.Println("  hop init    — pour reconfigurer")
		fmt.Println("  hop pair    — pour pairer une machine")
	},
}

func init() {
	rootCmd.AddCommand(resetCmd)
}
