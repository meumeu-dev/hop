package cmd

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var cfEnvFile string

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
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		var envContent string
		var domain string

		if cfEnvFile != "" {
			// Import from file or URL
			envContent, err = loadEnvFrom(cfEnvFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
				os.Exit(1)
			}
			// Extract domain from env content
			for _, line := range strings.Split(envContent, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "CF_DOMAIN=") {
					domain = strings.TrimPrefix(line, "CF_DOMAIN=")
				}
			}
			if domain == "" {
				fmt.Fprintln(os.Stderr, "CF_DOMAIN manquant dans le fichier .env")
				os.Exit(1)
			}
			fmt.Printf("→ Importe depuis %s\n", cfEnvFile)
		} else {
			// Interactive mode
			reader := bufio.NewReader(os.Stdin)

			current := cfg.Cloudflare.Domain
			if current != "" {
				fmt.Printf("Domaine Cloudflare [%s]: ", current)
			} else {
				fmt.Print("Domaine Cloudflare (ex: mondomaine.dev): ")
			}
			d, _ := reader.ReadString('\n')
			domain = strings.TrimSpace(d)
			if domain == "" {
				domain = current
			}
			if domain == "" {
				fmt.Fprintln(os.Stderr, "Domaine requis.")
				os.Exit(1)
			}

			fmt.Print("Email Cloudflare: ")
			email, _ := reader.ReadString('\n')
			email = strings.TrimSpace(email)

			fmt.Print("API Key Cloudflare (Global API Key ou token): ")
			apiKey, _ := reader.ReadString('\n')
			apiKey = strings.TrimSpace(apiKey)
			if apiKey == "" {
				fmt.Fprintln(os.Stderr, "Token API requis.")
				os.Exit(1)
			}

			envContent = fmt.Sprintf("CF_USER=%s\nCF_DOMAIN=%s\nCF_API_KEY=%s\n", email, domain, apiKey)
		}

		// Save domain in config
		cfg.Cloudflare = config.CloudflareConfig{
			Domain: domain,
		}

		// Save credentials
		envPath := filepath.Join(config.HopDir(), "cloudflare.env")
		if err := os.WriteFile(envPath, []byte(envContent), 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur ecriture %s: %v\n", envPath, err)
			os.Exit(1)
		}

		cfg.Cloudflare.EnvFile = envPath
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

// loadEnvFrom loads a .env file from a local path or URL
func loadEnvFrom(source string) (string, error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Get(source)
		if err != nil {
			return "", fmt.Errorf("telechargement: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	// Local file
	path := source
	if strings.HasPrefix(path, "~") {
		path = config.ExpandPath(path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("lecture %s: %w", source, err)
	}
	return string(data), nil
}

func init() {
	configCFCmd.Flags().StringVar(&cfEnvFile, "env", "", "Chemin ou URL vers un fichier .env (CF_USER, CF_DOMAIN, CF_API_KEY)")
	configCmd.AddCommand(configCFCmd)
	configCmd.AddCommand(configShowCmd)
	rootCmd.AddCommand(configCmd)
}
