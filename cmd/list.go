package cmd

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Liste les services et machines",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)

		if len(cfg.Services) > 0 {
			fmt.Println("Services:")
			keys := sortKeys(cfg.Services)
			for _, name := range keys {
				s := cfg.Services[name]
				fmt.Fprintf(w, "  %s\t%s\n", name, s.Desc)
			}
			w.Flush()
		}

		if len(cfg.Machines) > 0 {
			fmt.Println("\nMachines:")
			mkeys := sortKeys(cfg.Machines)
			for _, name := range mkeys {
				m := cfg.Machines[name]
				tunnel := ""
				if m.Tunnel != "" {
					tunnel = " | tunnel: " + m.Tunnel
				}
				svcs := ""
				if len(m.Services) > 0 {
					svcList := sortKeys(m.Services)
					svcs = " | services: "
					for i, s := range svcList {
						if i > 0 {
							svcs += ", "
						}
						svcs += s
					}
				}
				fmt.Fprintf(w, "  %s\t%s@%s%s%s\n", name, m.User, m.IP, tunnel, svcs)
			}
			w.Flush()
		}

		if len(cfg.Services) == 0 && len(cfg.Machines) == 0 {
			fmt.Println("Rien de configuré. Utilise 'hop add' pour commencer.")
		}
	},
}

func sortKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func init() {
	rootCmd.AddCommand(listCmd)
}
