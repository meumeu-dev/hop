package cmd

import (
	"fmt"
	"os"

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

		// Create permanent dir
		if err := os.MkdirAll(permanentDir, 0700); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		// Copy sandbox config to permanent if exists
		if _, err := os.Stat(sandboxDir); err == nil {
			entries, _ := os.ReadDir(sandboxDir)
			for _, entry := range entries {
				src := sandboxDir + "/" + entry.Name()
				dst := permanentDir + "/" + entry.Name()
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

		// Mark as installed
		os.WriteFile(permanentDir+"/.installed", []byte("installed\n"), 0600)

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
		s := src + "/" + entry.Name()
		d := dst + "/" + entry.Name()
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
