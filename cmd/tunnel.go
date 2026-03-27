package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	cf "github.com/meumeu-dev/hop/internal/cloudflared"
	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var tunnelCmd = &cobra.Command{
	Use:   "tunnel",
	Short: "Gere les tunnels Cloudflare",
}

var tunnelSetupCmd = &cobra.Command{
	Use:   "setup [nom]",
	Short: "Configure un tunnel Cloudflare sur cette machine",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if cfg.Cloudflare.EnvFile != "" {
			loadEnvFile(config.ExpandPath(cfg.Cloudflare.EnvFile))
		}

		cfPath, err := cf.EnsureInstalled()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		reader := bufio.NewReader(os.Stdin)

		tunnelName := ""
		if len(args) > 0 {
			tunnelName = args[0]
		} else {
			hostname, _ := os.Hostname()
			fmt.Printf("Nom du tunnel [%s]: ", hostname)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			if input == "" {
				tunnelName = hostname
			} else {
				tunnelName = input
			}
		}

		// Step 1: Login
		fmt.Println("\n→ Etape 1: Authentification Cloudflare")
		certPath := os.ExpandEnv("$HOME/.cloudflared/cert.pem")
		if _, err := os.Stat(certPath); os.IsNotExist(err) {
			if err := cf.Run("tunnel", "login"); err != nil {
				fmt.Fprintf(os.Stderr, "Erreur login: %v\n", err)
				os.Exit(1)
			}
		} else {
			fmt.Println("  Deja authentifie.")
		}

		// Step 2: Create tunnel
		fmt.Printf("\n→ Etape 2: Creation du tunnel '%s'\n", tunnelName)
		createCmd := exec.Command(cfPath, "tunnel", "create", "--", tunnelName)
		createCmd.Stdout = os.Stdout
		createCmd.Stderr = os.Stderr
		if err := createCmd.Run(); err != nil {
			fmt.Println("  Le tunnel existe peut-etre deja, on continue...")
		}

		// Step 3: Route DNS
		if cfg.Cloudflare.Domain != "" {
			hostname := tunnelName + "." + cfg.Cloudflare.Domain
			fmt.Printf("\n→ Etape 3: Route DNS %s\n", hostname)
			routeCmd := exec.Command(cfPath, "tunnel", "route", "dns", "--", tunnelName, hostname)
			routeCmd.Stdout = os.Stdout
			routeCmd.Stderr = os.Stderr
			if err := routeCmd.Run(); err != nil {
				fmt.Printf("  Route DNS peut-etre deja existante pour %s\n", hostname)
			}
		} else {
			fmt.Println("\n→ Etape 3: Pas de domaine configure, route DNS ignoree.")
			fmt.Println("  Configure ton domaine avec 'hop config'.")
		}

		// Step 4: Generate config
		fmt.Println("\n→ Etape 4: Generation de la config cloudflared")
		home, _ := os.UserHomeDir()
		cfConfigDir := filepath.Join(home, ".cloudflared")
		cfConfigPath := filepath.Join(cfConfigDir, "config.yml")

		listCmd := exec.Command(cfPath, "tunnel", "list", "-o", "json")
		listOut, err := listCmd.Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: impossible de lister les tunnels\n")
			os.Exit(1)
		}

		tunnelID := extractTunnelID(string(listOut))
		if tunnelID != "" && cfg.Cloudflare.Domain != "" {
			cfConfig := fmt.Sprintf("tunnel: %s\ncredentials-file: %s/%s.json\n\ningress:\n  - hostname: %s.%s\n    service: ssh://localhost:22\n  - service: http_status:404\n",
				tunnelID, cfConfigDir, tunnelID, tunnelName, cfg.Cloudflare.Domain)

			if err := os.WriteFile(cfConfigPath, []byte(cfConfig), 0600); err != nil {
				fmt.Fprintf(os.Stderr, "Erreur ecriture config: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("  Config ecrite: %s\n", cfConfigPath)
		}

		// Step 5: Run tunnel
		if config.IsInstalled() {
			fmt.Print("\n→ Installer comme service systemd (permanent) ? [o/N]: ")
			confirm, _ := reader.ReadString('\n')
			confirm = strings.TrimSpace(strings.ToLower(confirm))
			if confirm == "o" || confirm == "oui" || confirm == "y" || confirm == "yes" {
				serviceCmd := exec.Command("sudo", cfPath, "service", "install")
				serviceCmd.Stdin = os.Stdin
				serviceCmd.Stdout = os.Stdout
				serviceCmd.Stderr = os.Stderr
				if err := serviceCmd.Run(); err != nil {
					fmt.Printf("  Erreur. Lance manuellement: sudo %s service install\n", cfPath)
				} else {
					fmt.Println("  Service installe et demarre.")
				}
				fmt.Println("\n→ Tunnel permanent configure !")
				return
			}
		}

		// Foreground mode (default, sandbox-friendly)
		fmt.Println("\n→ Lancement du tunnel en foreground...")
		fmt.Println("  Ctrl+C pour arreter.")
		if !config.IsInstalled() {
			fmt.Println("  (hop install pour rendre permanent)")
		}
		fmt.Println()
		runCmd := exec.Command(cfPath, "tunnel", "run", "--", tunnelName)
		runCmd.Stdin = os.Stdin
		runCmd.Stdout = os.Stdout
		runCmd.Stderr = os.Stderr
		runCmd.Run()
	},
}

var tunnelStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Affiche l'etat des tunnels",
	Run: func(cmd *cobra.Command, args []string) {
		if !cf.IsInstalled() {
			fmt.Fprintln(os.Stderr, "cloudflared non installe. Lance: hop tunnel setup")
			os.Exit(1)
		}
		if err := cf.Run("tunnel", "list"); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	},
}

func extractTunnelID(jsonOutput string) string {
	var tunnels []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(jsonOutput), &tunnels); err == nil && len(tunnels) > 0 {
		return tunnels[0].ID
	}
	return ""
}

var allowedEnvPrefixes = []string{"CF_", "CLOUDFLARE_"}

func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		for _, prefix := range allowedEnvPrefixes {
			if strings.HasPrefix(strings.ToUpper(key), prefix) {
				os.Setenv(key, strings.TrimSpace(parts[1]))
				break
			}
		}
	}
}

// ── tunnel quick ─────────────────────────────────────────────────────────────

var tunnelQuickCmd = &cobra.Command{
	Use:   "quick",
	Short: "Lance un tunnel SSH rapide via Pinggy (zero install, zero compte)",
	Long: `Lance un tunnel TCP temporaire via Pinggy pour exposer le SSH de cette machine.
Zero install, zero compte requis. Timeout: 60 minutes (version gratuite).
Pour un tunnel permanent, utilise: hop tunnel setup (Cloudflare)`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runPinggyTunnel(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	},
}

// ── Pinggy ────────────────────────────────────────────────────────────────────

func runPinggyTunnel() error {
	fmt.Println()
	fmt.Println("→ Demarrage du tunnel Pinggy (via SSH)...")
	fmt.Println("  Zero install requis.")
	fmt.Println("  Timeout: 60 minutes (version gratuite).")
	fmt.Println("  Ctrl+C pour arreter.")
	fmt.Println()

	// ssh -p 443 -R0:localhost:22 tcp@a.pinggy.io
	// We capture stderr to detect the assigned URL, stdout goes to terminal
	sshArgs := []string{
		"-p", "443",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ServerAliveInterval=30",
		"-R", "0:localhost:22",
		"tcp@a.pinggy.io",
	}

	sshCmd := exec.Command("ssh", sshArgs...)

	// Pinggy prints the tunnel URL to stdout
	stdoutPipe, err := sshCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("pipe stdout: %w", err)
	}
	sshCmd.Stderr = os.Stderr
	sshCmd.Stdin = os.Stdin

	if err := sshCmd.Start(); err != nil {
		return fmt.Errorf("impossible de lancer ssh: %w", err)
	}

	// Parse output to find the tunnel URL
	// Pinggy outputs lines like: tcp://a.pinggy.io:XXXXX
	urlFound := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		reURL := regexp.MustCompile(`tcp://([a-zA-Z0-9._-]+:\d+)`)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Println(line)
			if m := reURL.FindStringSubmatch(line); m != nil {
				urlFound <- m[1]
			}
		}
		close(urlFound)
	}()

	// Wait up to 15s for URL to appear
	select {
	case hostPort, ok := <-urlFound:
		if ok && hostPort != "" {
			displayQuickTunnelInfo("Pinggy", hostPort)
		}
	case <-time.After(15 * time.Second):
		fmt.Println("  (URL non detectee automatiquement — verifiez la sortie ci-dessus)")
	}

	return sshCmd.Wait()
}

// ── display ───────────────────────────────────────────────────────────────────

func displayQuickTunnelInfo(provider, hostPort string) {
	// hostPort is "host:port"
	parts := strings.SplitN(hostPort, ":", 2)
	host := hostPort
	port := "22"
	if len(parts) == 2 {
		host = parts[0]
		port = parts[1]
	}

	hostname, _ := os.Hostname()
	user := os.Getenv("USER")
	if user == "" {
		user = "user"
	}

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────┐")
	fmt.Printf("│  Tunnel %s actif !                             \n", provider)
	fmt.Println("│")
	fmt.Printf("│  Adresse publique : %s\n", hostPort)
	fmt.Println("│")
	fmt.Printf("│  Connexion SSH    : ssh -p %s %s@%s\n", port, user, host)
	fmt.Println("│")
	fmt.Printf("│  hop config       : ajoute dans ~/.hop/config.yml\n")
	fmt.Printf("│    machines:\n")
	fmt.Printf("│      %s:\n", hostname)
	fmt.Printf("│        tunnel: %s\n", hostPort)
	fmt.Printf("│        user: %s\n", user)
	fmt.Println("│")
	fmt.Printf("│  Puis depuis une autre machine:\n")
	fmt.Printf("│    hop ssh %s\n", hostname)
	fmt.Println("└─────────────────────────────────────────────────────┘")
	fmt.Println()
}

func init() {
	tunnelCmd.AddCommand(tunnelSetupCmd)
	tunnelCmd.AddCommand(tunnelStatusCmd)
	tunnelCmd.AddCommand(tunnelQuickCmd)
	rootCmd.AddCommand(tunnelCmd)
}
