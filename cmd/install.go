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

var installYes bool

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Déploie la config et installe les outils sur ce PC",
	Run: func(cmd *cobra.Command, args []string) {
		dotfilesDir := filepath.Join(config.HopDir(), "dotfiles")
		home, _ := os.UserHomeDir()
		reader := bufio.NewReader(os.Stdin)

		entries, err := os.ReadDir(dotfilesDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur lecture dotfiles: %v\n", err)
			os.Exit(1)
		}

		if len(entries) > 0 {
			fmt.Println("Fichiers à déployer:")
			for _, entry := range entries {
				dst := filepath.Join(home, entry.Name())
				exists := ""
				if _, err := os.Stat(dst); err == nil {
					exists = " (ECRASE le fichier existant)"
				}
				fmt.Printf("  %s%s\n", entry.Name(), exists)
			}

			proceed := installYes
			if !proceed {
				fmt.Print("\nDéployer ces fichiers ? [o/N]: ")
				confirm, _ := reader.ReadString('\n')
				confirm = strings.TrimSpace(strings.ToLower(confirm))
				proceed = confirm == "o" || confirm == "oui" || confirm == "y" || confirm == "yes"
			}

			if proceed {
				for _, entry := range entries {
					src := filepath.Join(dotfilesDir, entry.Name())
					dst := filepath.Join(home, entry.Name())

					// Backup existing file
					if _, err := os.Stat(dst); err == nil {
						backupDst := dst + ".hop-backup"
						if data, err := os.ReadFile(dst); err == nil {
							os.WriteFile(backupDst, data, 0600)
						}
					}

					data, err := os.ReadFile(src)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Erreur lecture %s: %v\n", src, err)
						continue
					}

					if err := os.WriteFile(dst, data, 0600); err != nil {
						fmt.Fprintf(os.Stderr, "Erreur écriture %s: %v\n", dst, err)
						continue
					}

					fmt.Printf("  → %s\n", entry.Name())
				}
			} else {
				fmt.Println("→ Dotfiles ignorés.")
			}
		}

		installScript := filepath.Join(config.HopDir(), "install.sh")
		if _, err := os.Stat(installScript); err == nil {
			proceed := installYes
			if !proceed {
				fmt.Print("\nLancer install.sh ? [o/N]: ")
				confirm, _ := reader.ReadString('\n')
				confirm = strings.TrimSpace(strings.ToLower(confirm))
				proceed = confirm == "o" || confirm == "oui" || confirm == "y" || confirm == "yes"
			}

			if proceed {
				fmt.Println("\n→ Lancement script d'installation...")
				sh := exec.Command("bash", installScript)
				sh.Stdin = os.Stdin
				sh.Stdout = os.Stdout
				sh.Stderr = os.Stderr
				if err := sh.Run(); err != nil {
					fmt.Fprintf(os.Stderr, "Erreur install: %v\n", err)
					os.Exit(1)
				}
			}
		}

		fmt.Println("\n→ Installation terminée.")
	},
}

func init() {
	installCmd.Flags().BoolVarP(&installYes, "yes", "y", false, "Confirme automatiquement")
	rootCmd.AddCommand(installCmd)
}
