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

	"github.com/meumeu-dev/hop/internal/cfaccess"
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

		// Step 1: Load CF API credentials (no browser login needed)
		fmt.Println("\n→ Etape 1: Authentification Cloudflare (via API key)")
		cfEnv, err := cfaccess.LoadCFEnv(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			fmt.Fprintln(os.Stderr, "  Configure tes identifiants CF avec 'hop config' (CF_USER, CF_API_KEY, CF_ACCOUNT_ID dans cloudflare.env)")
			os.Exit(1)
		}
		fmt.Println("  API key chargee (pas de navigateur requis).")

		// Step 2: Create tunnel via CF API
		fmt.Printf("\n→ Etape 2: Creation du tunnel '%s' (via API)\n", tunnelName)
		tunnelInfo, err := cfaccess.CreateTunnel(cfEnv, tunnelName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur creation tunnel: %v\n", err)
			os.Exit(1)
		}
		tunnelID := tunnelInfo.ID
		if tunnelInfo.Secret != "" {
			fmt.Printf("  Tunnel cree (id: %s)\n", tunnelID)
		} else {
			fmt.Printf("  Tunnel existant (id: %s)\n", tunnelID)
		}

		// Step 3: Route DNS via CF API
		if cfg.Cloudflare.Domain != "" {
			hostname := tunnelName + "." + cfg.Cloudflare.Domain
			fmt.Printf("\n→ Etape 3: Route DNS %s (via API)\n", hostname)

			zoneID, err := cfaccess.GetZoneID(cfEnv, cfg.Cloudflare.Domain)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Erreur zone ID: %v\n", err)
				fmt.Fprintln(os.Stderr, "  Route DNS ignoree. Configurez manuellement le CNAME.")
			} else {
				if err := cfaccess.CreateDNSRecord(cfEnv, zoneID, hostname, tunnelID); err != nil {
					fmt.Fprintf(os.Stderr, "  Erreur DNS: %v\n", err)
				} else {
					fmt.Printf("  CNAME %s → %s.cfargotunnel.com\n", hostname, tunnelID)
				}
			}
		} else {
			fmt.Println("\n→ Etape 3: Pas de domaine configure, route DNS ignoree.")
			fmt.Println("  Configure ton domaine avec 'hop config'.")
		}

		// Step 4: Generate config + credentials file
		fmt.Println("\n→ Etape 4: Generation de la config cloudflared")
		home, _ := os.UserHomeDir()
		cfConfigDir := filepath.Join(home, ".cloudflared")
		cfConfigPath := filepath.Join(cfConfigDir, "config.yml")

		// Ensure .cloudflared directory exists
		if err := os.MkdirAll(cfConfigDir, 0700); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur creation dir %s: %v\n", cfConfigDir, err)
			os.Exit(1)
		}

		// Write tunnel credentials file (needed by cloudflared tunnel run)
		if tunnelInfo.Secret != "" {
			credsPath := filepath.Join(cfConfigDir, tunnelID+".json")
			creds := map[string]string{
				"AccountTag":   cfEnv.AccountID,
				"TunnelSecret": tunnelInfo.Secret,
				"TunnelID":     tunnelID,
			}
			credsJSON, _ := json.MarshalIndent(creds, "", "  ")
			if err := os.WriteFile(credsPath, credsJSON, 0600); err != nil {
				fmt.Fprintf(os.Stderr, "Erreur ecriture credentials: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("  Credentials ecrites: %s\n", credsPath)
		} else {
			// Tunnel already existed — check if credentials file is present
			credsPath := filepath.Join(cfConfigDir, tunnelID+".json")
			if _, err := os.Stat(credsPath); os.IsNotExist(err) {
				fmt.Printf("  ⚠ Fichier credentials absent: %s\n", credsPath)
				fmt.Println("  Le tunnel existait deja. Si les credentials sont perdues,")
				fmt.Println("  supprimez le tunnel dans le dashboard CF et relancez 'hop tunnel setup'.")
			}
		}

		if tunnelID != "" && cfg.Cloudflare.Domain != "" {
			cfConfig := fmt.Sprintf("tunnel: %s\ncredentials-file: %s/%s.json\n\ningress:\n  - hostname: %s.%s\n    service: ssh://localhost:22\n  - service: http_status:404\n",
				tunnelID, cfConfigDir, tunnelID, tunnelName, cfg.Cloudflare.Domain)

			if err := os.WriteFile(cfConfigPath, []byte(cfConfig), 0600); err != nil {
				fmt.Fprintf(os.Stderr, "Erreur ecriture config: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("  Config ecrite: %s\n", cfConfigPath)
		}

		// Step 5: CF Access setup (only when credentials are fully available)
		if cfg.Cloudflare.Domain != "" && cfg.Cloudflare.EnvFile != "" {
			fmt.Println("\n→ Etape 5: Configuration Cloudflare Access (service token)")
			result, err := cfaccess.Setup(cfg, tunnelName)
			if err != nil {
				fmt.Printf("  ⚠ Echec setup CF Access: %v\n", err)
				fmt.Println("  Le tunnel fonctionnera mais CF Access devra etre configure manuellement.")
			} else {
				if result.TokenID != "" {
					// Always update the token ID (may have been set already)
					cfg.Cloudflare.CFServiceTokenID = result.TokenID
				}
				if result.TokenSecret != "" {
					cfg.Cloudflare.CFServiceTokenSecret = result.TokenSecret
					fmt.Println("  → Service token secret sauvegarde dans la config.")
				} else if result.Reused {
					// Keep existing secret if we already have one
					fmt.Println("  → Token existant reutilise (secret conserve si deja present).")
				}
				if err := cfg.Save(); err != nil {
					fmt.Fprintf(os.Stderr, "  ⚠ Erreur sauvegarde config: %v\n", err)
				} else {
					fmt.Println("  → Config sauvegardee avec le service token.")
				}
			}
		} else {
			fmt.Println("\n→ Etape 5: CF Access ignore (EnvFile ou Domain non configure)")
			fmt.Println("  Relancez 'hop tunnel setup' apres 'hop config' pour configurer CF Access.")
		}

		// Step 6: Run tunnel (cloudflared needed here)
		cfPath, err := cf.EnsureInstalled()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur installation cloudflared: %v\n", err)
			os.Exit(1)
		}

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
