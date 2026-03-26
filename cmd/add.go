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
		enableTunnel, _ := cmd.Flags().GetBool("enable-tunnel")

		// Auto-generate tunnel hostname from domain if --tunnel is empty but --enable-tunnel is set
		if tunnel == "" && enableTunnel {
			cfg, err := config.Load()
			if err == nil && cfg.Cloudflare.Domain != "" {
				tunnel = name + "." + cfg.Cloudflare.Domain
				fmt.Printf("→ Tunnel auto: %s\n", tunnel)
			} else {
				fmt.Fprintln(os.Stderr, "Domaine Cloudflare non configuré. Lance 'hop init' ou 'hop dashboard' d'abord.")
				os.Exit(1)
			}
		}

		if err := config.ValidateName(name); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
		if err := config.ValidateIP(ip); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
		if err := config.ValidateUser(user); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
		if err := config.ValidateTunnel(tunnel); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

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

		if existing, ok := cfg.Machines[name]; ok {
			m.Services = existing.Services
		}

		cfg.Machines[name] = m

		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Machine '%s' ajoutee (%s@%s)\n", name, user, ip)
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
		tmux, _ := cmd.Flags().GetBool("tmux")
		session, _ := cmd.Flags().GetString("session")
		noperm, _ := cmd.Flags().GetBool("noperm")

		if err := config.ValidateName(name); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if svc, ok := cfg.Services[name]; ok && svc.Builtin {
			fmt.Fprintf(os.Stderr, "Impossible de modifier le service builtin '%s'.\n", name)
			os.Exit(1)
		}

		cfg.Services[name] = config.Service{
			Desc:    desc,
			Cmd:     command,
			Tmux:    tmux,
			Session: session,
			NoPerm:  noperm,
		}

		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Service '%s' ajouté.\n", name)
	},
}

// hop add <machine> <service> [--id <ID>] [--cmd <override>]
var addMachineServiceCmd = &cobra.Command{
	Use:   "<machine> <service>",
	Short: "Ajoute un service à une machine",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		machineName := args[0]
		serviceName := args[1]
		id, _ := cmd.Flags().GetString("id")
		customCmd, _ := cmd.Flags().GetString("cmd")

		if err := config.ValidateName(machineName); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
		if err := config.ValidateName(serviceName); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
		if id != "" {
			if err := config.ValidateRustdeskID(id); err != nil {
				fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
				os.Exit(1)
			}
		}

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		machine, ok := cfg.Machines[machineName]
		if !ok {
			fmt.Fprintf(os.Stderr, "Machine '%s' non trouvée.\n", machineName)
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
	addMachineCmd.Flags().Bool("enable-tunnel", false, "Active le tunnel (hostname auto-généré depuis le domaine)")
	addMachineCmd.MarkFlagRequired("user")

	addServiceCmd.Flags().String("cmd", "", "Commande à exécuter")
	addServiceCmd.Flags().String("desc", "", "Description")
	addServiceCmd.Flags().Bool("tmux", false, "Toujours lancer dans tmux")
	addServiceCmd.Flags().String("session", "", "Nom de session tmux par défaut")
	addServiceCmd.Flags().Bool("noperm", false, "Lance Claude sans permissions")
	addServiceCmd.MarkFlagRequired("cmd")

	addMachineServiceCmd.Flags().String("id", "", "ID Rustdesk")
	addMachineServiceCmd.Flags().String("cmd", "", "Commande custom")

	addCmd.AddCommand(addMachineCmd)
	addCmd.AddCommand(addServiceCmd)
	addCmd.AddCommand(addMachineServiceCmd)
	rootCmd.AddCommand(addCmd)
}
