package cmd

import (
	"fmt"
	"os"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Ajoute une machine ou un service",
}

// hop add machine <nom> <ip> --user <user> [--tunnel <hostname>]
var addMachineCmd = &cobra.Command{
	Use:   "machine <nom> <ip>",
	Short: "Ajoute une machine",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		ip := args[1]
		user, _ := cmd.Flags().GetString("user")
		tunnel, _ := cmd.Flags().GetString("tunnel")

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		m := config.Machine{
			IP:       ip,
			User:     user,
			Tunnel:   tunnel,
			Services: make(map[string]config.MachineService),
		}

		// Keep existing services if machine already exists
		if existing, ok := cfg.Machines[name]; ok {
			m.Services = existing.Services
		}

		cfg.Machines[name] = m

		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Machine '%s' ajoutée (%s@%s)\n", name, user, ip)
	},
}

// hop add service <nom> --cmd <commande> [--desc <description>]
var addServiceCmd = &cobra.Command{
	Use:   "service <nom>",
	Short: "Ajoute un service (commande réutilisable)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		command, _ := cmd.Flags().GetString("cmd")
		desc, _ := cmd.Flags().GetString("desc")

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		cfg.Services[name] = config.Service{
			Desc: desc,
			Cmd:  command,
		}

		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Service '%s' ajouté.\n", name)
	},
}

// hop add <machine> rustdesk --id <ID>
// hop add <machine> <service> [--cmd <override>]
var addMachineServiceCmd = &cobra.Command{
	Use:   "<machine> <service>",
	Short: "Ajoute un service à une machine",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		machineName := args[0]
		serviceName := args[1]
		id, _ := cmd.Flags().GetString("id")
		customCmd, _ := cmd.Flags().GetString("cmd")

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		machine, ok := cfg.Machines[machineName]
		if !ok {
			fmt.Fprintf(os.Stderr, "Machine '%s' non trouvée. Ajoute-la d'abord: hop add machine %s <ip> --user <user>\n", machineName, machineName)
			os.Exit(1)
		}

		if machine.Services == nil {
			machine.Services = make(map[string]config.MachineService)
		}

		machine.Services[serviceName] = config.MachineService{
			ID:  id,
			Cmd: customCmd,
		}
		cfg.Machines[machineName] = machine

		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Service '%s' ajouté à '%s'.\n", serviceName, machineName)
	},
}

func init() {
	addMachineCmd.Flags().String("user", "", "Utilisateur SSH")
	addMachineCmd.Flags().String("tunnel", "", "Hostname Cloudflare Tunnel")
	addMachineCmd.MarkFlagRequired("user")

	addServiceCmd.Flags().String("cmd", "", "Commande à exécuter")
	addServiceCmd.Flags().String("desc", "", "Description")
	addServiceCmd.MarkFlagRequired("cmd")

	addMachineServiceCmd.Flags().String("id", "", "ID Rustdesk")
	addMachineServiceCmd.Flags().String("cmd", "", "Commande custom")

	addCmd.AddCommand(addMachineCmd)
	addCmd.AddCommand(addServiceCmd)
	addCmd.AddCommand(addMachineServiceCmd)
	rootCmd.AddCommand(addCmd)
}
