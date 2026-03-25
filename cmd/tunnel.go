package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var tunnelCmd = &cobra.Command{
	Use:   "tunnel",
	Short: "Gère les tunnels Cloudflare",
}

var tunnelSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure un tunnel Cloudflare sur cette machine",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if cfg.Cloudflare.EnvFile != "" {
			loadEnvFile(config.ExpandPath(cfg.Cloudflare.EnvFile))
		}

		if _, err := exec.LookPath("cloudflared"); err != nil {
			fmt.Println("cloudflared non trouvé. Installation...")
			sh := exec.Command("bash", "-c",
				"curl -sL https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64 -o /tmp/cloudflared && chmod +x /tmp/cloudflared && sudo mv /tmp/cloudflared /usr/local/bin/")
			sh.Stdin = os.Stdin
			sh.Stdout = os.Stdout
			sh.Stderr = os.Stderr
			if err := sh.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Erreur installation cloudflared: %v\n", err)
				os.Exit(1)
			}
		}

		fmt.Println("→ Lancement de 'cloudflared tunnel login'...")
		login := exec.Command("cloudflared", "tunnel", "login")
		login.Stdin = os.Stdin
		login.Stdout = os.Stdout
		login.Stderr = os.Stderr
		if err := login.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur login: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("→ Tunnel configuré.")
	},
}

var tunnelStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Affiche l'état des tunnels",
	Run: func(cmd *cobra.Command, args []string) {
		sh := exec.Command("cloudflared", "tunnel", "list")
		sh.Stdout = os.Stdout
		sh.Stderr = os.Stderr
		if err := sh.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "cloudflared non disponible ou non configuré.\n")
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
		// Only allow safe env var prefixes
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
