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
	"github.com/meumeu-dev/hop/internal/pairing"
	"github.com/spf13/cobra"
)

var cfEnvFile string
var configShow bool

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configure hop (Cloudflare + worker)",
	Long: `Sans flag: configure Cloudflare (domaine + token API).
Avec --show: affiche la config actuelle.
Avec --env: importe un fichier .env.

hop config                           # interactif
hop config --env ~/token-cf.env      # import fichier
hop config --env https://...         # import URL
hop config --show                    # affiche la config`,
	Run: func(cmd *cobra.Command, args []string) {
		if configShow {
			runConfigShow()
			return
		}
		runConfigCF()
	},
}

func runConfigShow() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
		os.Exit(1)
	}

	if config.IsInstalled() {
		fmt.Println("Mode:      installe (~/.hop/)")
	} else {
		fmt.Printf("Mode:      sandbox (%s)\n", config.HopDir())
	}

	fmt.Printf("Machines:  %d\n", len(cfg.Machines))
	fmt.Printf("Services:  %d\n", len(cfg.Services))
	fmt.Printf("Aliases:   %d\n", len(cfg.Aliases))

	if cfg.Cloudflare.Domain != "" {
		fmt.Printf("CF domain: %s\n", cfg.Cloudflare.Domain)
	} else {
		fmt.Println("CF domain: (non configure) — hop config pour configurer")
	}

	if cfg.WorkerURL != "" {
		fmt.Printf("Worker:    %s\n", cfg.WorkerURL)
	} else {
		fmt.Printf("Worker:    %s (defaut)\n", pairing.DefaultWorkerURL)
	}
}

func runConfigCF() {
	// Ensure hop dir exists
	if err := config.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
		os.Exit(1)
	}

	if config.IsInstalled() {
		fmt.Println("Mode: installe (~/.hop/)")
	} else {
		fmt.Printf("Mode: sandbox (%s) — disparait au reboot\n", config.HopDir())
		fmt.Println("  hop install pour rendre permanent")
	}
	fmt.Println()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
		os.Exit(1)
	}

	var envContent string
	var domain string

	if cfEnvFile != "" {
		envContent, err = loadEnvFrom(cfEnvFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
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

		fmt.Print("API Key Cloudflare: ")
		apiKey, _ := reader.ReadString('\n')
		apiKey = strings.TrimSpace(apiKey)
		if apiKey == "" {
			fmt.Fprintln(os.Stderr, "Token API requis.")
			os.Exit(1)
		}

		fmt.Print("Account ID Cloudflare (optionnel, pour Workers AI): ")
		accountID, _ := reader.ReadString('\n')
		accountID = strings.TrimSpace(accountID)

		envContent = fmt.Sprintf("CF_USER=%s\nCF_DOMAIN=%s\nCF_API_KEY=%s\n", email, domain, apiKey)
		if accountID != "" {
			envContent += fmt.Sprintf("CF_ACCOUNT_ID=%s\n", accountID)
		}

		// Ask for custom worker URL
		fmt.Printf("Worker relay [%s]: ", pairing.DefaultWorkerURL)
		workerInput, _ := reader.ReadString('\n')
		workerInput = strings.TrimSpace(workerInput)
		if workerInput != "" && strings.HasPrefix(workerInput, "https://") {
			cfg.WorkerURL = workerInput
		}
	}

	cfg.Cloudflare = config.CloudflareConfig{
		Domain: domain,
	}

	envPath := filepath.Join(config.HopDir(), "cloudflare.env")
	if err := os.WriteFile(envPath, []byte(envContent), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur ecriture %s: %v\n", envPath, err)
		os.Exit(1)
	}

	cfg.Cloudflare.EnvFile = envPath
	cfg.Save()

	fmt.Println()
	fmt.Printf("→ Cloudflare configure: %s\n", domain)
	if cfg.WorkerURL != "" {
		fmt.Printf("→ Worker: %s\n", cfg.WorkerURL)
	}
	fmt.Println()
	fmt.Println("Prochaines etapes:")
	fmt.Println("  hop tunnel setup       — creer un tunnel")
	fmt.Println("  hop pair               — pairer une machine")
}

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
	configCmd.Flags().BoolVar(&configShow, "show", false, "Affiche la config actuelle")
	configCmd.Flags().StringVar(&cfEnvFile, "env", "", "Chemin ou URL vers un fichier .env")
	rootCmd.AddCommand(configCmd)
}
