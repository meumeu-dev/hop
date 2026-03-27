package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configure hop",
}

var configCFCmd = &cobra.Command{
	Use:   "cf",
	Short: "Configure Cloudflare (domaine + token API)",
	Long: `Configure ton compte Cloudflare pour les tunnels et le worker.

Necessite:
  - Un compte Cloudflare (gratuit: dash.cloudflare.com)
  - Un domaine ajoute dans Cloudflare
  - Un token API: dash.cloudflare.com/profile/api-tokens
    → Template "Edit zone DNS" ou Global API Key`,
	Run: func(cmd *cobra.Command, args []string) {
		reader := bufio.NewReader(os.Stdin)

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		// Domain
		current := cfg.Cloudflare.Domain
		if current != "" {
			fmt.Printf("Domaine Cloudflare [%s]: ", current)
		} else {
			fmt.Print("Domaine Cloudflare (ex: mondomaine.dev): ")
		}
		domain, _ := reader.ReadString('\n')
		domain = strings.TrimSpace(domain)
		if domain == "" {
			domain = current
		}
		if domain == "" {
			fmt.Fprintln(os.Stderr, "Domaine requis.")
			os.Exit(1)
		}

		// Email
		fmt.Print("Email Cloudflare: ")
		email, _ := reader.ReadString('\n')
		email = strings.TrimSpace(email)

		// API Key
		fmt.Print("API Key Cloudflare (Global API Key ou token): ")
		apiKey, _ := reader.ReadString('\n')
		apiKey = strings.TrimSpace(apiKey)
		if apiKey == "" {
			fmt.Fprintln(os.Stderr, "Token API requis.")
			os.Exit(1)
		}

		// Save domain in config
		cfg.Cloudflare = config.CloudflareConfig{
			Domain: domain,
		}
		cfg.Save()

		// Save credentials in cloudflare.env (secured, gitignored)
		envContent := fmt.Sprintf("CF_USER=%s\nCF_DOMAIN=%s\nCF_API_KEY=%s\n", email, domain, apiKey)
		envPath := config.HopDir() + "/cloudflare.env"
		if err := os.WriteFile(envPath, []byte(envContent), 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur ecriture %s: %v\n", envPath, err)
			os.Exit(1)
		}

		// Also store env_file reference in config
		cfg.Cloudflare.EnvFile = "~/.hop/cloudflare.env"
		cfg.Save()

		fmt.Println()
		fmt.Printf("→ Cloudflare configure: %s\n", domain)
		fmt.Printf("→ Credentials dans %s\n", envPath)
		fmt.Println()
		fmt.Println("Prochaines etapes:")
		fmt.Println("  hop tunnel setup       — creer un tunnel sur cette machine")
		fmt.Println("  hop pair               — pairer avec une autre machine")
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Affiche la config actuelle",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Machines:  %d\n", len(cfg.Machines))
		fmt.Printf("Services:  %d\n", len(cfg.Services))
		fmt.Printf("Aliases:   %d\n", len(cfg.Aliases))

		if cfg.Cloudflare.Domain != "" {
			fmt.Printf("CF domain: %s\n", cfg.Cloudflare.Domain)
		} else {
			fmt.Println("CF domain: (non configure)")
		}

		if cfg.WorkerURL != "" {
			fmt.Printf("Worker:    %s\n", cfg.WorkerURL)
		} else {
			fmt.Println("Worker:    par defaut")
		}
	},
}

func init() {
	configCmd.AddCommand(configCFCmd)
	configCmd.AddCommand(configShowCmd)
	rootCmd.AddCommand(configCmd)
}
