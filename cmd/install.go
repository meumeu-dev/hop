package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Rend hop permanent (config persistante, survit au reboot)",
	Long: `Par defaut hop est en mode sandbox: la config est dans /tmp/ et disparait au reboot.
hop install copie la config dans ~/.hop/ et la rend persistante.`,
	Run: func(cmd *cobra.Command, args []string) {
		sandboxDir := config.HopDir()
		permanentDir := config.PermanentDir()

		if config.IsInstalled() {
			fmt.Println("hop est deja installe.")
			fmt.Printf("Config: %s\n", permanentDir)
			return
		}

		// Detect legacy install (old ~/.hop/ without .installed marker)
		legacyConfig := filepath.Join(permanentDir, "config.yml")
		legacyMarker := filepath.Join(permanentDir, ".installed")
		if _, err := os.Stat(legacyConfig); err == nil {
			if _, err := os.Stat(legacyMarker); os.IsNotExist(err) {
				fmt.Println("⚠ Ancienne installation detectee dans ~/.hop/")
				fmt.Println("  Elle sera supprimee et remplacee par la config sandbox actuelle.")
				reader := bufio.NewReader(os.Stdin)
				fmt.Print("Continuer ? [o/N]: ")
				confirm, _ := reader.ReadString('\n')
				confirm = strings.TrimSpace(strings.ToLower(confirm))
				if confirm != "o" && confirm != "oui" && confirm != "y" && confirm != "yes" {
					fmt.Println("Annule.")
					return
				}
				os.RemoveAll(permanentDir)
				fmt.Println("→ Ancienne config supprimee.")
			}
		}

		// Create permanent dir
		if err := os.MkdirAll(permanentDir, 0700); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		// Verify sandbox dir is not a symlink (security)
		if fi, err := os.Lstat(sandboxDir); err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				fmt.Fprintln(os.Stderr, "Erreur: le dossier sandbox est un lien symbolique (possible attaque)")
				os.Exit(1)
			}
			// Copy sandbox config to permanent
			entries, _ := os.ReadDir(sandboxDir)
			for _, entry := range entries {
				src := filepath.Join(sandboxDir, entry.Name())
				dst := filepath.Join(permanentDir, entry.Name())
				if entry.IsDir() {
					copyDir(src, dst)
				} else {
					data, err := os.ReadFile(src)
					if err == nil {
						os.WriteFile(dst, data, 0600)
					}
				}
			}
			// Remove sandbox
			os.RemoveAll(sandboxDir)
		}

		// Mark as installed (do this last — atomic state transition)
		os.WriteFile(filepath.Join(permanentDir, ".installed"), []byte("installed\n"), 0600)

		fmt.Println("→ hop installe en mode permanent")
		fmt.Printf("  Config: %s\n", permanentDir)
		fmt.Println("  La config survit au reboot.")
		fmt.Println("  hop uninstall pour tout supprimer.")
	},
}

func copyDir(src, dst string) {
	os.MkdirAll(dst, 0700)
	entries, _ := os.ReadDir(src)
	for _, entry := range entries {
		s := filepath.Join(src, entry.Name())
		d := filepath.Join(dst, entry.Name())
		// Skip symlinks
		if fi, err := os.Lstat(s); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if entry.IsDir() {
			copyDir(s, d)
		} else {
			data, err := os.ReadFile(s)
			if err == nil {
				os.WriteFile(d, data, 0600)
			}
		}
	}
}

func init() {
	rootCmd.AddCommand(installCmd)
}
