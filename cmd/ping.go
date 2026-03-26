package cmd

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var pingCmd = &cobra.Command{
	Use:   "ping [machine]",
	Short: "Verifie l'etat des machines",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if len(args) == 1 {
			name := cfg.ResolveAlias(args[0])
			m, ok := cfg.Machines[name]
			if !ok {
				fmt.Fprintf(os.Stderr, "Machine '%s' non trouvee.\n", name)
				os.Exit(1)
			}
			status, latency := pingMachine(m)
			printPingResult(name, m, status, latency)
			if status == "offline" {
				os.Exit(1)
			}
			return
		}

		// Ping all machines
		if len(cfg.Machines) == 0 {
			fmt.Println("Aucune machine configuree.")
			return
		}

		allOk := true
		for name, m := range cfg.Machines {
			status, latency := pingMachine(m)
			printPingResult(name, m, status, latency)
			if status == "offline" {
				allOk = false
			}
		}
		if !allOk {
			os.Exit(1)
		}
	},
}

func pingMachine(m config.Machine) (status string, latency time.Duration) {
	if m.IP == "" {
		return "no-ip", 0
	}
	start := time.Now()
	conn, err := net.DialTimeout("tcp", m.IP+":22", 2*time.Second)
	if err != nil {
		return "offline", 0
	}
	conn.Close()
	return "online", time.Since(start)
}

func printPingResult(name string, m config.Machine, status string, latency time.Duration) {
	switch status {
	case "online":
		fmt.Printf("  %-15s %-16s %s (%s)\n", name, m.IP, "✓ en ligne", latency.Round(time.Millisecond))
	case "offline":
		fmt.Printf("  %-15s %-16s %s\n", name, m.IP, "✗ hors ligne")
	case "no-ip":
		fmt.Printf("  %-15s %-16s %s\n", name, "(pas d'IP)", "- tunnel uniquement")
	}
}

func init() {
	rootCmd.AddCommand(pingCmd)
}
