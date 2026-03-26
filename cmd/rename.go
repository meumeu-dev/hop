package cmd

import (
	"fmt"
	"os"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var renameCmd = &cobra.Command{
	Use:   "rename <ancien> <nouveau>",
	Short: "Renomme une machine ou un service",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		oldName := args[0]
		newName := args[1]

		if err := config.ValidateName(newName); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		renamed := false

		// Try machine
		if m, ok := cfg.Machines[oldName]; ok {
			if _, exists := cfg.Machines[newName]; exists {
				fmt.Fprintf(os.Stderr, "Machine '%s' existe deja.\n", newName)
				os.Exit(1)
			}
			cfg.Machines[newName] = m
			delete(cfg.Machines, oldName)
			fmt.Printf("Machine '%s' renommee en '%s'.\n", oldName, newName)
			renamed = true
		}

		// Try service
		if s, ok := cfg.Services[oldName]; ok {
			if s.Builtin {
				fmt.Fprintf(os.Stderr, "Impossible de renommer le service builtin '%s'.\n", oldName)
				os.Exit(1)
			}
			if _, exists := cfg.Services[newName]; exists {
				fmt.Fprintf(os.Stderr, "Service '%s' existe deja.\n", newName)
				os.Exit(1)
			}
			cfg.Services[newName] = s
			delete(cfg.Services, oldName)
			fmt.Printf("Service '%s' renomme en '%s'.\n", oldName, newName)
			renamed = true
		}

		if !renamed {
			fmt.Fprintf(os.Stderr, "'%s' non trouve.\n", oldName)
			os.Exit(1)
		}

		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(renameCmd)
}
