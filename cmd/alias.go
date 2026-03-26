package cmd

import (
	"fmt"
	"os"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var aliasCmd = &cobra.Command{
	Use:   "alias",
	Short: "Gere les alias (raccourcis de noms)",
}

var aliasAddCmd = &cobra.Command{
	Use:   "add <alias> <cible>",
	Short: "Cree un alias",
	Long:  "Cree un raccourci. Ex: hop alias add rpi raspberrypi",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		alias := args[0]
		target := args[1]

		if err := config.ValidateName(alias); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if cfg.Aliases == nil {
			cfg.Aliases = make(map[string]string)
		}

		cfg.Aliases[alias] = target

		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Alias '%s' -> '%s'\n", alias, target)
	},
}

var aliasRemoveCmd = &cobra.Command{
	Use:   "remove <alias>",
	Short: "Supprime un alias",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		alias := args[0]

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if cfg.Aliases == nil {
			fmt.Fprintf(os.Stderr, "Alias '%s' non trouve.\n", alias)
			os.Exit(1)
		}

		if _, ok := cfg.Aliases[alias]; !ok {
			fmt.Fprintf(os.Stderr, "Alias '%s' non trouve.\n", alias)
			os.Exit(1)
		}

		delete(cfg.Aliases, alias)

		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Alias '%s' supprime.\n", alias)
	},
}

var aliasListCmd = &cobra.Command{
	Use:   "list",
	Short: "Liste les alias",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if len(cfg.Aliases) == 0 {
			fmt.Println("Aucun alias configure.")
			return
		}

		fmt.Println("Alias:")
		for alias, target := range cfg.Aliases {
			fmt.Printf("  %s -> %s\n", alias, target)
		}
	},
}

func init() {
	aliasCmd.AddCommand(aliasAddCmd)
	aliasCmd.AddCommand(aliasRemoveCmd)
	aliasCmd.AddCommand(aliasListCmd)
	rootCmd.AddCommand(aliasCmd)
}
