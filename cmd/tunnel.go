package cmd

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
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
	Short: "Lance un tunnel rapide (Pinggy ou ngrok) sans configuration permanente",
	Long: `Lance un tunnel TCP rapide pour exposer le SSH de cette machine.

Providers disponibles:
  1) Pinggy  — zero install, SSH natif, gratuit, timeout 60min
  2) ngrok   — necessite le binaire ngrok, gratuit, session 2h, 1GB/mois`,
	Run: func(cmd *cobra.Command, args []string) {
		runTunnelQuick()
	},
}

func runTunnelQuick() {
	fmt.Println("┌─────────────────────────────────────────────┐")
	fmt.Println("│         hop tunnel quick                    │")
	fmt.Println("│  Tunnel SSH temporaire — zero config        │")
	fmt.Println("└─────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("Choix du provider:")
	fmt.Println("  1) Pinggy  [defaut] — zero install, SSH, gratuit, 60min")
	fmt.Println("  2) ngrok            — binaire requis, gratuit, 2h, 1GB/mois")
	fmt.Println()
	fmt.Print("Provider [1]: ")

	reader := bufio.NewReader(os.Stdin)
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	switch choice {
	case "", "1", "pinggy":
		if err := runPinggyTunnel(); err != nil {
			fmt.Fprintf(os.Stderr, "\nErreur Pinggy: %v\n", err)
			fmt.Println("\nEssai avec ngrok...")
			if err2 := runNgrokTunnel(); err2 != nil {
				fmt.Fprintf(os.Stderr, "Erreur ngrok: %v\n", err2)
				os.Exit(1)
			}
		}
	case "2", "ngrok":
		if err := runNgrokTunnel(); err != nil {
			fmt.Fprintf(os.Stderr, "\nErreur ngrok: %v\n", err)
			fmt.Println("\nEssai avec Pinggy...")
			if err2 := runPinggyTunnel(); err2 != nil {
				fmt.Fprintf(os.Stderr, "Erreur Pinggy: %v\n", err2)
				os.Exit(1)
			}
		}
	default:
		fmt.Fprintln(os.Stderr, "Choix invalide. Utilise 1 (Pinggy) ou 2 (ngrok).")
		os.Exit(1)
	}
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
		"-o", "StrictHostKeyChecking=no",
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

// ── ngrok ─────────────────────────────────────────────────────────────────────

func ngrokBinPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hop", "bin", "ngrok")
}

func ngrokURL() string {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		return "https://bin.equinox.io/c/bNyj1mQVY4c/ngrok-v3-stable-linux-amd64.tgz"
	case "arm64":
		return "https://bin.equinox.io/c/bNyj1mQVY4c/ngrok-v3-stable-linux-arm64.tgz"
	case "arm":
		return "https://bin.equinox.io/c/bNyj1mQVY4c/ngrok-v3-stable-linux-arm.tgz"
	default:
		return "https://bin.equinox.io/c/bNyj1mQVY4c/ngrok-v3-stable-linux-amd64.tgz"
	}
}

func ensureNgrok() (string, error) {
	binPath := ngrokBinPath()

	// Check system ngrok first
	if systemNgrok, err := exec.LookPath("ngrok"); err == nil {
		return systemNgrok, nil
	}

	// Check ~/.hop/bin/ngrok
	if _, err := os.Stat(binPath); err == nil {
		return binPath, nil
	}

	// Auto-install
	fmt.Println("  ngrok non trouve. Installation automatique dans ~/.hop/bin/ngrok...")
	url := ngrokURL()
	fmt.Printf("  Telechargement: %s\n", url)

	if err := os.MkdirAll(filepath.Dir(binPath), 0755); err != nil {
		return "", fmt.Errorf("impossible de creer ~/.hop/bin: %w", err)
	}

	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("echec telechargement ngrok: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("telechargement ngrok: HTTP %d", resp.StatusCode)
	}

	// Extract tgz
	gr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return "", fmt.Errorf("decompression: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	found := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("lecture archive: %w", err)
		}
		if hdr.Name == "ngrok" || strings.HasSuffix(hdr.Name, "/ngrok") {
			f, err := os.OpenFile(binPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				return "", fmt.Errorf("ecriture ngrok: %w", err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return "", fmt.Errorf("ecriture ngrok: %w", err)
			}
			f.Close()
			found = true
			break
		}
	}

	if !found {
		return "", fmt.Errorf("binaire ngrok non trouve dans l'archive")
	}

	fmt.Printf("  ngrok installe: %s\n", binPath)
	return binPath, nil
}

func runNgrokTunnel() error {
	fmt.Println()
	fmt.Println("→ Demarrage du tunnel ngrok...")

	ngrokBin, err := ensureNgrok()
	if err != nil {
		return fmt.Errorf("ngrok: %w", err)
	}

	fmt.Println("  Timeout: ~2h (version gratuite), 1GB/mois.")
	fmt.Println("  Ctrl+C pour arreter.")
	fmt.Println()

	// ngrok tcp 22 --log stdout --log-format json
	ngrokArgs := []string{"tcp", "22", "--log", "stdout", "--log-format", "json"}
	ngrokCmd := exec.Command(ngrokBin, ngrokArgs...)

	stdoutPipe, err := ngrokCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("pipe stdout: %w", err)
	}
	ngrokCmd.Stderr = os.Stderr
	ngrokCmd.Stdin = os.Stdin

	if err := ngrokCmd.Start(); err != nil {
		return fmt.Errorf("impossible de lancer ngrok: %w", err)
	}

	// Parse JSON log output to find tunnel URL
	// ngrok logs: {"addr":"tcp://0.tcp.ngrok.io:XXXXX","url":"tcp://0.tcp.ngrok.io:XXXXX",...}
	urlFound := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		reURL := regexp.MustCompile(`"url"\s*:\s*"tcp://([a-zA-Z0-9._-]+:\d+)"`)
		reAddr := regexp.MustCompile(`"Addr"\s*:\s*"([a-zA-Z0-9._-]+:\d+)"`)
		for scanner.Scan() {
			line := scanner.Text()
			// Only print non-JSON lines or key info to avoid clutter
			if !strings.HasPrefix(strings.TrimSpace(line), "{") {
				fmt.Println(line)
			}
			if m := reURL.FindStringSubmatch(line); m != nil {
				urlFound <- m[1]
			} else if m := reAddr.FindStringSubmatch(line); m != nil {
				urlFound <- m[1]
			}
		}
		close(urlFound)
	}()

	// Wait up to 15s for URL to appear
	select {
	case hostPort, ok := <-urlFound:
		if ok && hostPort != "" {
			displayQuickTunnelInfo("ngrok", hostPort)
		}
	case <-time.After(15 * time.Second):
		fmt.Println("  (URL non detectee automatiquement — verifiez la sortie ci-dessus)")
	}

	return ngrokCmd.Wait()
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
