package cmd

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "hop <service> [machine]",
	Short: "hop — ton lanceur de commandes, SSH et config perso",
	Args:  cobra.ArbitraryArgs,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if err := config.Init(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur init: %v\n", err)
			os.Exit(1)
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			cmd.Help()
			return
		}

		serviceName := args[0]
		var machineName string
		if len(args) > 1 {
			machineName = args[1]
		}

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		service, serviceOk := cfg.Services[serviceName]

		if !serviceOk {
			fmt.Fprintf(os.Stderr, "Service '%s' non trouvé. Utilise 'hop list' pour voir les services.\n", serviceName)
			os.Exit(1)
		}

		// No machine → run locally
		if machineName == "" {
			if service.Builtin {
				fmt.Fprintf(os.Stderr, "'%s' nécessite une machine. Ex: hop %s pc1\n", serviceName, serviceName)
				os.Exit(1)
			}
			runLocal(service.Cmd)
			return
		}

		// With machine
		machine, machineOk := cfg.Machines[machineName]
		if !machineOk {
			fmt.Fprintf(os.Stderr, "Machine '%s' non trouvée. Utilise 'hop list' pour voir les machines.\n", machineName)
			os.Exit(1)
		}

		switch serviceName {
		case "ssh":
			runSSH(machine)
		case "rustdesk":
			runRustdesk(machine, machineName)
		default:
			runRemote(machine, service, serviceName)
		}
	},
}

func detectTarget(m config.Machine) (target string, viaTunnel bool) {
	if m.IP != "" {
		conn, err := net.DialTimeout("tcp", m.IP+":22", 500*time.Millisecond)
		if err == nil {
			conn.Close()
			fmt.Printf("→ Connexion locale (%s)\n", m.IP)
			return m.User + "@" + m.IP, false
		}
	}

	if m.Tunnel != "" {
		fmt.Printf("→ Connexion via Cloudflare Tunnel (%s)\n", m.Tunnel)
		return m.User + "@" + m.Tunnel, true
	}

	fmt.Fprintln(os.Stderr, "Aucune connexion disponible.")
	os.Exit(1)
	return "", false
}

func sshArgs(target string, viaTunnel bool) []string {
	args := []string{}
	if viaTunnel {
		args = append(args, "-o", "ProxyCommand=cloudflared access ssh --hostname %h")
	}
	args = append(args, target)
	return args
}

func runSSH(m config.Machine) {
	target, viaTunnel := detectTarget(m)
	args := sshArgs(target, viaTunnel)
	sh := exec.Command("ssh", args...)
	sh.Stdin = os.Stdin
	sh.Stdout = os.Stdout
	sh.Stderr = os.Stderr
	err := sh.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
	}
}

func runRustdesk(m config.Machine, name string) {
	ms, ok := m.Services["rustdesk"]
	if !ok || ms.ID == "" {
		fmt.Fprintf(os.Stderr, "Rustdesk non configuré pour '%s'. Utilise: hop add %s rustdesk --id <ID>\n", name, name)
		os.Exit(1)
	}
	if err := config.ValidateRustdeskID(ms.ID); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
		os.Exit(1)
	}
	sh := exec.Command("rustdesk", "--connect", ms.ID)
	sh.Stdin = os.Stdin
	sh.Stdout = os.Stdout
	sh.Stderr = os.Stderr
	sh.Start()
	fmt.Printf("→ Rustdesk lancé vers %s (%s)\n", name, ms.ID)
}

func runRemote(m config.Machine, svc config.Service, name string) {
	target, viaTunnel := detectTarget(m)

	// Check if machine has a custom cmd for this service
	remoteCmd := svc.Cmd
	if ms, ok := m.Services[name]; ok && ms.Cmd != "" {
		remoteCmd = ms.Cmd
	}

	args := sshArgs(target, viaTunnel)
	args = append(args, "-t", "--", remoteCmd)
	sh := exec.Command("ssh", args...)
	sh.Stdin = os.Stdin
	sh.Stdout = os.Stdout
	sh.Stderr = os.Stderr
	err := sh.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
	}
}

func runLocal(command string) {
	sh := exec.Command("bash", "-c", command)
	sh.Stdin = os.Stdin
	sh.Stdout = os.Stdout
	sh.Stderr = os.Stderr
	err := sh.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
	}
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
