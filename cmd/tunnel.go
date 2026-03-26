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
	Short: "Gère les tunnels Cloudflare",
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

		// Auto-install cloudflared
		cfPath, err := cf.EnsureInstalled()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		reader := bufio.NewReader(os.Stdin)

		// Determine tunnel name
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

		// Step 1: Login if needed
		fmt.Println("\n→ Étape 1: Authentification Cloudflare")
		certPath := os.ExpandEnv("$HOME/.cloudflared/cert.pem")
		if _, err := os.Stat(certPath); os.IsNotExist(err) {
			if err := cf.Run("tunnel", "login"); err != nil {
				fmt.Fprintf(os.Stderr, "Erreur login: %v\n", err)
				os.Exit(1)
			}
		} else {
			fmt.Println("  Déjà authentifié.")
		}

		// Step 2: Create tunnel
		fmt.Printf("\n→ Étape 2: Création du tunnel '%s'\n", tunnelName)
		createCmd := exec.Command(cfPath, "tunnel", "create", tunnelName)
		createCmd.Stdout = os.Stdout
		createCmd.Stderr = os.Stderr
		if err := createCmd.Run(); err != nil {
			fmt.Println("  Le tunnel existe peut-être déjà, on continue...")
		}

		// Step 3: Route DNS
		if cfg.Cloudflare.Domain != "" {
			hostname := tunnelName + "." + cfg.Cloudflare.Domain
			fmt.Printf("\n→ Étape 3: Route DNS %s\n", hostname)
			routeCmd := exec.Command(cfPath, "tunnel", "route", "dns", tunnelName, hostname)
			routeCmd.Stdout = os.Stdout
			routeCmd.Stderr = os.Stderr
			if err := routeCmd.Run(); err != nil {
				fmt.Printf("  Route DNS peut-être déjà existante pour %s\n", hostname)
			}
		} else {
			fmt.Println("\n→ Étape 3: Pas de domaine configuré, route DNS ignorée.")
			fmt.Println("  Configure ton domaine avec 'hop init' ou 'hop dashboard'.")
		}

		// Step 4: Generate config
		fmt.Println("\n→ Étape 4: Génération de la config cloudflared")
		cfConfigDir := os.ExpandEnv("$HOME/.cloudflared")
		cfConfigPath := cfConfigDir + "/config.yml"

		// Find tunnel UUID
		listCmd := exec.Command(cfPath, "tunnel", "list", "-o", "json")
		listOut, err := listCmd.Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: impossible de lister les tunnels\n")
			os.Exit(1)
		}

		// Simple extraction of tunnel ID (avoid importing encoding/json just for this)
		tunnelID := ""
		lines := strings.Split(string(listOut), "\"")
		for i, part := range lines {
			if part == "id" && i+2 < len(lines) {
				tunnelID = lines[i+2]
				break
			}
		}

		if tunnelID != "" {
			cfConfig := fmt.Sprintf(`tunnel: %s
credentials-file: %s/%s.json

ingress:
  - hostname: %s.%s
    service: ssh://localhost:22
  - service: http_status:404
`, tunnelID, cfConfigDir, tunnelID, tunnelName, cfg.Cloudflare.Domain)

			if err := os.WriteFile(cfConfigPath, []byte(cfConfig), 0600); err != nil {
				fmt.Fprintf(os.Stderr, "Erreur écriture config: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("  Config écrite: %s\n", cfConfigPath)
		}

		// Step 5: Ask to install as service
		fmt.Print("\n→ Installer cloudflared comme service systemd ? [o/N]: ")
		confirm, _ := reader.ReadString('\n')
		confirm = strings.TrimSpace(strings.ToLower(confirm))
		if confirm == "o" || confirm == "oui" || confirm == "y" || confirm == "yes" {
			serviceCmd := exec.Command("sudo", cfPath, "service", "install")
			serviceCmd.Stdin = os.Stdin
			serviceCmd.Stdout = os.Stdout
			serviceCmd.Stderr = os.Stderr
			if err := serviceCmd.Run(); err != nil {
				fmt.Println("  Erreur installation service. Tu peux lancer manuellement:")
				fmt.Printf("  sudo %s service install\n", cfPath)
			} else {
				fmt.Println("  Service installé et démarré.")
			}
		} else {
			fmt.Println("  Pour lancer manuellement:")
			fmt.Printf("  %s tunnel run %s\n", cfPath, tunnelName)
		}

		fmt.Println("\n→ Tunnel configuré !")
	},
}

var tunnelStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Affiche l'état des tunnels",
	Run: func(cmd *cobra.Command, args []string) {
		if !cf.IsInstalled() {
			fmt.Fprintln(os.Stderr, "cloudflared non installé. Lance: hop tunnel setup")
			os.Exit(1)
		}
		if err := cf.Run("tunnel", "list"); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	},
}

// allowedEnvPrefixes limits which env vars can be set from the env file
var allowedEnvPrefixes = []string{"CF_", "CLOUDFLARE_", "TUNNEL_"}

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
		allowed := false
		for _, prefix := range allowedEnvPrefixes {
			if strings.HasPrefix(strings.ToUpper(key), prefix) {
				allowed = true
				break
			}
		}
		if allowed {
			os.Setenv(key, strings.TrimSpace(parts[1]))
		}
	}
}

func init() {
	tunnelCmd.AddCommand(tunnelSetupCmd)
	tunnelCmd.AddCommand(tunnelStatusCmd)
	rootCmd.AddCommand(tunnelCmd)
}
