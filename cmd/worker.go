package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Gere le worker de pairing",
}

var workerDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploie le worker hop-pair sur ton compte Cloudflare",
	Long: `Deploie automatiquement le worker de pairing sur ton compte Cloudflare.
Necessite: npm/npx installe + credentials Cloudflare configurees.

Apres le deploiement, hop utilisera ton propre worker au lieu du worker par defaut.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Check npx
		if _, err := exec.LookPath("npx"); err != nil {
			fmt.Fprintln(os.Stderr, "Erreur: npx requis (installe Node.js)")
			os.Exit(1)
		}

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		// Load CF env if configured
		if cfg.Cloudflare.EnvFile != "" {
			loadWorkerEnv(config.ExpandPath(cfg.Cloudflare.EnvFile))
		}

		// Find worker source
		workerDir := findWorkerDir()
		if workerDir == "" {
			fmt.Fprintln(os.Stderr, "Erreur: impossible de trouver les sources du worker.")
			fmt.Fprintln(os.Stderr, "Clone le repo: git clone https://github.com/meumeu-dev/hop.git")
			os.Exit(1)
		}

		fmt.Printf("→ Deploiement du worker depuis %s...\n", workerDir)

		// Run wrangler deploy
		deployCmd := exec.Command("npx", "wrangler", "deploy")
		deployCmd.Dir = workerDir
		deployCmd.Stdin = os.Stdin
		deployCmd.Stdout = os.Stdout
		deployCmd.Stderr = os.Stderr

		if err := deployCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur deploiement: %v\n", err)
			os.Exit(1)
		}

		// Get the worker URL
		fmt.Println()
		fmt.Println("→ Worker deploye !")
		fmt.Println()
		fmt.Println("Pour utiliser ton worker, configure l'URL dans hop:")
		fmt.Println("  Ajoute dans ~/.hop/config.yml:")
		fmt.Println("  worker_url: https://hop-pair.<ton-sous-domaine>.workers.dev")
	},
}

var workerURLCmd = &cobra.Command{
	Use:   "url [url]",
	Short: "Affiche ou configure l'URL du worker",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if len(args) == 0 {
			if cfg.WorkerURL != "" {
				fmt.Printf("Worker custom: %s\n", cfg.WorkerURL)
			} else {
				fmt.Println("Worker par defaut: https://hop-pair.meumeudev.workers.dev")
			}
			return
		}

		url := args[0]
		if url == "default" || url == "reset" {
			cfg.WorkerURL = ""
			fmt.Println("→ Worker remis par defaut.")
		} else {
			if !strings.HasPrefix(url, "https://") {
				fmt.Fprintln(os.Stderr, "Erreur: l'URL doit commencer par https://")
				os.Exit(1)
			}
			cfg.WorkerURL = url
			fmt.Printf("→ Worker configure: %s\n", url)
		}

		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	},
}

func findWorkerDir() string {
	// Check relative to hop binary
	execPath, _ := os.Executable()
	if execPath != "" {
		dir := filepath.Join(filepath.Dir(execPath), "..", "worker")
		if _, err := os.Stat(filepath.Join(dir, "worker.js")); err == nil {
			return dir
		}
	}

	// Check current directory
	if _, err := os.Stat("worker/worker.js"); err == nil {
		return "worker"
	}

	// Check home
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, "hop", "worker"),
		filepath.Join(home, "meumeudev", "projets", "hop", "worker"),
	}
	for _, d := range candidates {
		if _, err := os.Stat(filepath.Join(d, "worker.js")); err == nil {
			return d
		}
	}

	return ""
}

func loadWorkerEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			if strings.HasPrefix(key, "CF_") || strings.HasPrefix(key, "CLOUDFLARE_") {
				os.Setenv(key, strings.TrimSpace(parts[1]))
			}
		}
	}
}

func init() {
	workerCmd.AddCommand(workerDeployCmd)
	workerCmd.AddCommand(workerURLCmd)
	rootCmd.AddCommand(workerCmd)
}
