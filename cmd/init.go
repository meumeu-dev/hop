package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/meumeu-dev/hop/internal/dashboard"
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
			exec.Command("git", "config", "--global", "user.name", "--", gitName).Run()
		} else {
			fmt.Printf("Git name: %s\n", gitName)
		}

		if gitEmail == "" {
			fmt.Print("Ton email (pour git): ")
			gitEmail, _ = reader.ReadString('\n')
			gitEmail = strings.TrimSpace(gitEmail)
			exec.Command("git", "config", "--global", "user.email", "--", gitEmail).Run()
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

		// Ask CLI or Dashboard
		fmt.Println("Comment veux-tu configurer hop ?")
		fmt.Println("  1) Dashboard (navigateur)")
		fmt.Println("  2) CLI (terminal)")
		fmt.Print("Choix [1/2]: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		if choice == "1" {
			fmt.Println()
			fmt.Println("→ Lancement du dashboard...")
			dashboard.Start(8080, true)
		} else {
			fmt.Println()
			fmt.Println("Prochaines étapes:")
			fmt.Println("  hop add machine rpi 192.168.1.50 --user pi")
			fmt.Println("  hop add service claude --cmd \"claude --dangerously-skip-permissions\" --desc \"Claude\"")
			fmt.Println("  hop list")
			fmt.Println("  hop dashboard   — pour ouvrir le dashboard plus tard")
		}
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

		reader := bufio.NewReader(os.Stdin)

		// Warn about service commands in cloned config
		if clonedCfg, err := config.Load(); err == nil {
			hasCustomCmds := false
			for name, svc := range clonedCfg.Services {
				if !svc.Builtin && svc.Cmd != "" {
					if !hasCustomCmds {
						fmt.Println("\n⚠ Services avec commandes personnalisées détectés:")
						hasCustomCmds = true
					}
					fmt.Printf("  %s: %s\n", name, svc.Cmd)
				}
			}
			if hasCustomCmds {
				fmt.Println("  Ces commandes seront exécutées quand tu lanceras ces services.")
			}
		}

		// Deploy dotfiles with confirmation
		dotfilesDir := filepath.Join(hopDir, "dotfiles")
		home, _ := os.UserHomeDir()
		entries, _ := os.ReadDir(dotfilesDir)

		if len(entries) > 0 {
			fmt.Println("\nFichiers à déployer:")
			for _, entry := range entries {
				dst := filepath.Join(home, entry.Name())
				exists := ""
				if _, err := os.Stat(dst); err == nil {
					exists = " (ECRASE le fichier existant)"
				}
				fmt.Printf("  %s%s\n", entry.Name(), exists)
			}
			fmt.Print("\nDéployer ces fichiers ? [o/N]: ")
			confirm, _ := reader.ReadString('\n')
			confirm = strings.TrimSpace(strings.ToLower(confirm))

			if confirm == "o" || confirm == "oui" || confirm == "y" || confirm == "yes" {
				for _, entry := range entries {
					src := filepath.Join(dotfilesDir, entry.Name())
					dst := filepath.Join(home, entry.Name())

					// Backup existing file
					if _, err := os.Stat(dst); err == nil {
						backupDst := dst + ".hop-backup"
						if data, err := os.ReadFile(dst); err == nil {
							os.WriteFile(backupDst, data, 0600)
							fmt.Printf("  → backup: %s.hop-backup\n", entry.Name())
						}
					}

					data, err := os.ReadFile(src)
					if err != nil {
						continue
					}
					os.WriteFile(dst, data, 0600)
					fmt.Printf("  → %s\n", entry.Name())
				}
			} else {
				fmt.Println("→ Dotfiles ignorés.")
			}
		}

		// Run install script with confirmation
		installScript := filepath.Join(hopDir, "install.sh")
		if _, err := os.Stat(installScript); err == nil {
			fmt.Println("\n→ Script d'installation trouvé: install.sh")
			// Show first 20 lines
			if data, err := os.ReadFile(installScript); err == nil {
				lines := strings.Split(string(data), "\n")
				fmt.Println("--- contenu ---")
				for _, line := range lines {
					fmt.Println("  " + line)
				}
				fmt.Println("--- fin ---")
			}
			fmt.Print("Exécuter ce script ? [o/N]: ")
			confirm, _ := reader.ReadString('\n')
			confirm = strings.TrimSpace(strings.ToLower(confirm))

			if confirm == "o" || confirm == "oui" || confirm == "y" || confirm == "yes" {
				sh := exec.Command("bash", installScript)
				sh.Stdin = os.Stdin
				sh.Stdout = os.Stdout
				sh.Stderr = os.Stderr
				sh.Run()
			} else {
				fmt.Println("→ Script ignoré.")
			}
		}

		fmt.Println()
		fmt.Println("→ hop est prêt ! Tape 'hop list' pour voir ta config.")
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(cloneCmd)
}
