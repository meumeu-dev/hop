package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

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
			fmt.Println("  Configure ton domaine avec 'hop config cf'.")
		}

		// Step 4: Generate config
		fmt.Println("\n→ Etape 4: Generation de la config cloudflared")
		cfConfigDir := os.ExpandEnv("$HOME/.cloudflared")
		cfConfigPath := cfConfigDir + "/config.yml"

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
	parts := strings.Split(jsonOutput, "\"")
	for i, part := range parts {
		if part == "id" && i+2 < len(parts) {
			return parts[i+2]
		}
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

func init() {
	tunnelCmd.AddCommand(tunnelSetupCmd)
	tunnelCmd.AddCommand(tunnelStatusCmd)
	rootCmd.AddCommand(tunnelCmd)
}
