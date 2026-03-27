package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var resetYes bool

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Remet la config a zero (garde le binaire)",
	Run: func(cmd *cobra.Command, args []string) {
		hopDir := config.HopDir()

		if _, err := os.Stat(hopDir); os.IsNotExist(err) {
			fmt.Printf("Rien a reset, %s n'existe pas.\n", hopDir)
			return
		}

		if !resetYes {
			fmt.Printf("Ceci va supprimer toute la config dans %s\n", hopDir)
			fmt.Println("(machines, services, cles SSH, secrets)")
			fmt.Println()

			reader := bufio.NewReader(os.Stdin)
			fmt.Print("Confirmer le reset ? [oui/N]: ")
			confirm, _ := reader.ReadString('\n')
			confirm = strings.TrimSpace(strings.ToLower(confirm))

			if confirm != "oui" && confirm != "yes" {
				fmt.Println("Annule.")
				return
			}
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
	resetCmd.Flags().BoolVarP(&resetYes, "yes", "y", false, "Skip la confirmation")
	rootCmd.AddCommand(resetCmd)
}
