package cmd

import (
	"fmt"
	"os"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:   "remove <nom> [service]",
	Short: "Supprime une machine, un service, ou un service d'une machine",
	Args:  cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if len(args) == 2 {
			// Remove service from machine
			serviceName := args[1]
			machine, ok := cfg.Machines[name]
			if !ok {
				fmt.Fprintf(os.Stderr, "Machine '%s' non trouvée.\n", name)
				os.Exit(1)
			}
			delete(machine.Services, serviceName)
			cfg.Machines[name] = machine
			fmt.Printf("Service '%s' supprimé de '%s'.\n", serviceName, name)
		} else {
			// Remove machine or service
			_, isMachine := cfg.Machines[name]
			_, isService := cfg.Services[name]

			if !isMachine && !isService {
				fmt.Fprintf(os.Stderr, "'%s' non trouvé.\n", name)
				os.Exit(1)
			}

			if isMachine {
				delete(cfg.Machines, name)
				fmt.Printf("Machine '%s' supprimée.\n", name)
			}
			if isService {
				delete(cfg.Services, name)
				fmt.Printf("Service '%s' supprimé.\n", name)
			}
		}

		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(removeCmd)
}
