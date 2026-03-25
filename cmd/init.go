package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Configure hop depuis zéro (wizard)",
	Run: func(cmd *cobra.Command, args []string) {
		reader := bufio.NewReader(os.Stdin)

		fmt.Println("=== hop init ===")
		fmt.Println()

		// Check git config
		nameOut, _ := exec.Command("git", "config", "--global", "user.name").Output()
		emailOut, _ := exec.Command("git", "config", "--global", "user.email").Output()

		gitName := strings.TrimSpace(string(nameOut))
		gitEmail := strings.TrimSpace(string(emailOut))

		if gitName == "" {
			fmt.Print("Ton nom (pour git): ")
			gitName, _ = reader.ReadString('\n')
			gitName = strings.TrimSpace(gitName)
			exec.Command("git", "config", "--global", "user.name", gitName).Run()
		} else {
			fmt.Printf("Git name: %s\n", gitName)
		}

		if gitEmail == "" {
			fmt.Print("Ton email (pour git): ")
			gitEmail, _ = reader.ReadString('\n')
			gitEmail = strings.TrimSpace(gitEmail)
			exec.Command("git", "config", "--global", "user.email", gitEmail).Run()
		} else {
			fmt.Printf("Git email: %s\n", gitEmail)
		}

		fmt.Println()

		// Init hop dir
		if err := config.Init(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		hopDir := config.HopDir()

		// Init git repo in ~/.hop
		gitDir := filepath.Join(hopDir, ".git")
		if _, err := os.Stat(gitDir); os.IsNotExist(err) {
			fmt.Println("→ Initialisation du repo git dans ~/.hop...")
			exec.Command("git", "-C", hopDir, "init").Run()
		}

		// Ask for Cloudflare config
		fmt.Print("Domaine Cloudflare (laisser vide pour skip): ")
		domain, _ := reader.ReadString('\n')
		domain = strings.TrimSpace(domain)

		cfg, _ := config.Load()

		if domain != "" {
			fmt.Print("Chemin vers le fichier token CF (ex: ~/token-cf.env): ")
			envFile, _ := reader.ReadString('\n')
			envFile = strings.TrimSpace(envFile)

			cfg.Cloudflare = config.CloudflareConfig{
				Domain:  domain,
				EnvFile: envFile,
			}
		}

		cfg.Save()

		// Ask for remote repo
		fmt.Print("URL du repo distant pour sync (laisser vide pour skip): ")
		remote, _ := reader.ReadString('\n')
		remote = strings.TrimSpace(remote)

		if remote != "" {
			exec.Command("git", "-C", hopDir, "remote", "add", "origin", remote).Run()
			fmt.Printf("→ Remote ajouté: %s\n", remote)
		}

		// Create default install.sh
		installPath := filepath.Join(hopDir, "install.sh")
		if _, err := os.Stat(installPath); os.IsNotExist(err) {
			defaultInstall := `#!/bin/bash
# Script d'installation des outils — lancé par 'hop install'
echo "=== Installation des outils ==="

# hop
if ! command -v hop &>/dev/null; then
    echo "Installation de hop..."
    # TODO: mettre l'URL du binaire hop quand il sera distribué
fi

# tmux
if ! command -v tmux &>/dev/null; then
    echo "Installation de tmux..."
    sudo apt install tmux -y 2>/dev/null || sudo yum install tmux -y 2>/dev/null
fi

echo "=== Installation terminée ==="
`
			os.WriteFile(installPath, []byte(defaultInstall), 0755)
		}

		// Initial commit
		exec.Command("git", "-C", hopDir, "add", "-A").Run()
		exec.Command("git", "-C", hopDir, "commit", "-m", "hop init").Run()

		fmt.Println()
		fmt.Println("→ hop est prêt !")
		fmt.Println()
		fmt.Println("Prochaines étapes:")
		fmt.Println("  hop add machine rpi 192.168.1.50 --user pi")
		fmt.Println("  hop add service claude --cmd \"claude --dangerously-skip-permissions\" --desc \"Claude\"")
		fmt.Println("  hop list")
	},
}

var cloneCmd = &cobra.Command{
	Use:   "clone <repo>",
	Short: "Clone une config hop existante et installe tout",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		repo := args[0]
		hopDir := config.HopDir()

		// Expand short GitHub format
		if !strings.Contains(repo, "://") && !strings.HasPrefix(repo, "git@") {
			repo = "https://github.com/" + repo + ".git"
		}

		// Check if ~/.hop already exists
		if _, err := os.Stat(filepath.Join(hopDir, "config.yml")); err == nil {
			fmt.Fprintf(os.Stderr, "~/.hop existe déjà. Supprime-le d'abord si tu veux repartir de zéro.\n")
			os.Exit(1)
		}

		// Clone
		fmt.Printf("→ Clonage de %s...\n", repo)
		sh := exec.Command("git", "clone", repo, hopDir)
		sh.Stdout = os.Stdout
		sh.Stderr = os.Stderr
		if err := sh.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur clone: %v\n", err)
			os.Exit(1)
		}

		// Deploy dotfiles
		dotfilesDir := filepath.Join(hopDir, "dotfiles")
		home, _ := os.UserHomeDir()
		entries, _ := os.ReadDir(dotfilesDir)

		if len(entries) > 0 {
			fmt.Println("→ Déploiement des dotfiles...")
			for _, entry := range entries {
				src := filepath.Join(dotfilesDir, entry.Name())
				dst := filepath.Join(home, entry.Name())
				data, err := os.ReadFile(src)
				if err != nil {
					continue
				}
				os.WriteFile(dst, data, 0644)
				fmt.Printf("  → %s\n", entry.Name())
			}
		}

		// Run install script
		installScript := filepath.Join(hopDir, "install.sh")
		if _, err := os.Stat(installScript); err == nil {
			fmt.Println("→ Lancement du script d'installation...")
			sh := exec.Command("bash", installScript)
			sh.Stdin = os.Stdin
			sh.Stdout = os.Stdout
			sh.Stderr = os.Stderr
			sh.Run()
		}

		fmt.Println()
		fmt.Println("→ hop est prêt ! Tape 'hop list' pour voir ta config.")
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(cloneCmd)
}
