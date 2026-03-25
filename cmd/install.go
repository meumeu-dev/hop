package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Déploie la config et installe les outils sur ce PC",
	Run: func(cmd *cobra.Command, args []string) {
		dotfilesDir := filepath.Join(config.HopDir(), "dotfiles")
		home, _ := os.UserHomeDir()

		entries, err := os.ReadDir(dotfilesDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur lecture dotfiles: %v\n", err)
			os.Exit(1)
		}

		for _, entry := range entries {
			src := filepath.Join(dotfilesDir, entry.Name())
			dst := filepath.Join(home, entry.Name())

			data, err := os.ReadFile(src)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Erreur lecture %s: %v\n", src, err)
				continue
			}

			if err := os.WriteFile(dst, data, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Erreur écriture %s: %v\n", dst, err)
				continue
			}

			fmt.Printf("  → %s\n", entry.Name())
		}

		installScript := filepath.Join(config.HopDir(), "install.sh")
		if _, err := os.Stat(installScript); err == nil {
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

		fmt.Println("\n→ Installation terminée.")
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
}
