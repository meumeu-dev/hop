package cmd

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/meumeu-dev/hop/internal/cloudflared"
	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var tmuxFlag bool
var sessionFlag string
var noPermFlag bool

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

		// Resolve aliases
		serviceName = cfg.ResolveAlias(serviceName)
		if machineName != "" {
			machineName = cfg.ResolveAlias(machineName)
		}

		service, serviceOk := cfg.Services[serviceName]

		if !serviceOk {
			fmt.Fprintf(os.Stderr, "Service '%s' non trouvé. Utilise 'hop list' pour voir les services.\n", serviceName)
			os.Exit(1)
		}

		// Determine options: flags override config
		useTmux := tmuxFlag || service.Tmux
		sessionName := sessionFlag
		if sessionName == "" {
			sessionName = service.Session
		}
		useNoPerm := noPermFlag || service.NoPerm

		// Inject --dangerously-skip-permissions for claude commands
		if useNoPerm && isClaudeCmd(service.Cmd) && !strings.Contains(service.Cmd, "--dangerously-skip-permissions") {
			service.Cmd = service.Cmd + " --dangerously-skip-permissions"
		}

		// No machine → run locally
		if machineName == "" {
			if service.Builtin {
				fmt.Fprintf(os.Stderr, "'%s' nécessite une machine. Ex: hop %s pc1\n", serviceName, serviceName)
				os.Exit(1)
			}

			if useTmux {
				runLocalTmux(service, serviceName, sessionName)
			} else {
				runLocal(service.Cmd)
			}
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
			if useTmux {
				runSSHTmux(machine, sessionName)
			} else {
				runSSH(machine)
			}
		case "rustdesk":
			runRustdesk(machine, machineName)
		default:
			if useTmux {
				runRemoteTmux(machine, service, serviceName, sessionName)
			} else {
				runRemote(machine, service, serviceName)
			}
		}
	},
}

// askSession prompts for a session name if not provided
func askSession(sessionName string, serviceName string) string {
	if sessionName != "" {
		return sessionName
	}
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Nom de session tmux [%s]: ", serviceName)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return serviceName
	}
	return input
}

// isClaudeCmd detects if the command is a Claude CLI command
func isClaudeCmd(cmd string) bool {
	return strings.Contains(cmd, "claude")
}

// buildTmuxCmd wraps a command in tmux
func buildTmuxCmd(command string, session string) *exec.Cmd {
	// If it's a claude command, add --session-name
	if isClaudeCmd(command) && !strings.Contains(command, "--session-name") {
		command = command + " --session-name " + session
	}
	return exec.Command("tmux", "new-session", "-s", session, command)
}

func runLocalTmux(svc config.Service, serviceName string, sessionName string) {
	session := askSession(sessionName, serviceName)
	sh := buildTmuxCmd(svc.Cmd, session)
	sh.Stdin = os.Stdin
	sh.Stdout = os.Stdout
	sh.Stderr = os.Stderr
	fmt.Printf("→ tmux session '%s'\n", session)
	err := sh.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
	}
}

func runSSHTmux(m config.Machine, sessionName string) {
	session := askSession(sessionName, "ssh")
	target, viaTunnel := detectTarget(m)
	args := sshArgs(target, viaTunnel)

	// SSH into remote, then start tmux there
	remoteCmd := fmt.Sprintf("tmux new-session -A -s %s", session)
	args = append(args, "-t", "--", remoteCmd)
	sh := exec.Command("ssh", args...)
	sh.Stdin = os.Stdin
	sh.Stdout = os.Stdout
	sh.Stderr = os.Stderr
	fmt.Printf("→ tmux session '%s' (distant)\n", session)
	err := sh.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
	}
}

func runRemoteTmux(m config.Machine, svc config.Service, serviceName string, sessionName string) {
	session := askSession(sessionName, serviceName)
	target, viaTunnel := detectTarget(m)

	remoteCmd := svc.Cmd
	if ms, ok := m.Services[serviceName]; ok && ms.Cmd != "" {
		remoteCmd = ms.Cmd
	}

	// If it's a claude command, add --session-name
	if isClaudeCmd(remoteCmd) && !strings.Contains(remoteCmd, "--session-name") {
		remoteCmd = remoteCmd + " --session-name " + session
	}

	// Wrap in tmux on the remote
	tmuxRemoteCmd := fmt.Sprintf("tmux new-session -s %s '%s'", session, strings.ReplaceAll(remoteCmd, "'", "'\\''"))

	args := sshArgs(target, viaTunnel)
	args = append(args, "-t", "--", tmuxRemoteCmd)
	sh := exec.Command("ssh", args...)
	sh.Stdin = os.Stdin
	sh.Stdout = os.Stdout
	sh.Stderr = os.Stderr
	fmt.Printf("→ tmux session '%s' (distant)\n", session)
	err := sh.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
	}
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
		// Auto-install cloudflared if needed
		if _, err := cloudflared.EnsureInstalled(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			fmt.Fprintln(os.Stderr, "Installe cloudflared avec: hop tunnel setup")
			os.Exit(1)
		}
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
		cfPath := cloudflared.Path()
		args = append(args, "-o", fmt.Sprintf("ProxyCommand=%s access ssh --hostname %%h", cfPath))
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

func init() {
	rootCmd.Flags().BoolVar(&tmuxFlag, "tmux", false, "Lance dans tmux")
	rootCmd.Flags().StringVarP(&sessionFlag, "session", "s", "", "Nom de la session tmux")
	rootCmd.Flags().BoolVar(&noPermFlag, "noperm", false, "Lance Claude sans permissions")
}
