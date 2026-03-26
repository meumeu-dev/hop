package cmd

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/meumeu-dev/hop/internal/cloudflared"
	"github.com/meumeu-dev/hop/internal/config"
	"github.com/meumeu-dev/hop/internal/tunnel"
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
		// Silent update check (once per day, non-blocking)
		if cmd.Name() != "update" && cmd.Name() != "version" && cmd.Name() != "uninstall" {
			go CheckUpdateBackground()
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
		sessionName := sanitizeSession(sessionFlag)
		if sessionName == "" {
			sessionName = sanitizeSession(service.Session)
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

var validSessionName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// sanitizeSession ensures session name is safe for tmux/shell
func sanitizeSession(name string) string {
	if validSessionName.MatchString(name) {
		return name
	}
	// Replace unsafe chars
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '-'
	}, name)
	return safe
}

// askSession prompts for a session name if not provided
func askSession(sessionName string, serviceName string) string {
	if sessionName != "" {
		return sanitizeSession(sessionName)
	}
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Nom de session tmux [%s]: ", serviceName)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return sanitizeSession(serviceName)
	}
	return sanitizeSession(input)
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
	// 1. Try LAN
	if m.IP != "" {
		conn, err := net.DialTimeout("tcp", m.IP+":22", 500*time.Millisecond)
		if err == nil {
			conn.Close()
			fmt.Printf("→ Connexion locale (%s)\n", m.IP)
			return m.User + "@" + m.IP, false
		}
	}

	// 2. Try configured tunnel
	if m.Tunnel != "" {
		if _, err := cloudflared.EnsureInstalled(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			fmt.Fprintln(os.Stderr, "Installe cloudflared avec: hop tunnel setup")
			os.Exit(1)
		}
		fmt.Printf("→ Connexion via Cloudflare Tunnel (%s)\n", m.Tunnel)
		return m.User + "@" + m.Tunnel, true
	}

	// 3. Try dynamic tunnel via worker
	hostname := ""
	cfg, _ := config.Load()
	if cfg != nil {
		for name, machine := range cfg.Machines {
			if machine.IP == m.IP && machine.User == m.User {
				hostname = name
				break
			}
		}
	}
	if hostname != "" {
		if dynURL, err := tunnel.Resolve(hostname); err == nil && dynURL != "" {
			// bore tunnel: direct SSH to host:port
			fmt.Printf("→ Connexion via tunnel dynamique (%s)\n", dynURL)
			// Parse host:port for SSH
			if strings.Contains(dynURL, ":") {
				parts := strings.SplitN(dynURL, ":", 2)
				sshHost := parts[0]
				sshPort := parts[1]
				fmt.Printf("→ SSH vers %s port %s\n", sshHost, sshPort)
				args := []string{"-p", sshPort, m.User + "@" + sshHost}
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
				os.Exit(0)
			}
			return m.User + "@" + dynURL, false
		}
	}

	fmt.Fprintln(os.Stderr, "Aucune connexion disponible.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "La machine n'est pas joignable en LAN et aucun tunnel n'est configure.")
	fmt.Fprintln(os.Stderr, "Options:")
	fmt.Fprintln(os.Stderr, "  - Sur la machine distante: hop tunnel quick  (tunnel temporaire)")
	fmt.Fprintln(os.Stderr, "  - Sur la machine distante: hop tunnel setup  (tunnel permanent)")
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
