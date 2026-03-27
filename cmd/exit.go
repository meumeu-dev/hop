package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var exitCmd = &cobra.Command{
	Use:   "exit",
	Short: "Supprime toute trace de hop (config + binaire)",
	Long: `Supprime la config, les cles, et le binaire hop. Zero trace sur le systeme.
En mode installe (hop install), utilise hop uninstall a la place.`,
	Run: func(cmd *cobra.Command, args []string) {
		if config.IsInstalled() {
			fmt.Println("hop est en mode installe.")
			fmt.Println("Utilise 'hop uninstall' pour tout supprimer.")
			os.Exit(1)
		}

		// Remove config
		hopDir := config.HopDir()
		if _, err := os.Stat(hopDir); err == nil {
			os.RemoveAll(hopDir)
		}

		// Remove binary
		execPath, _ := os.Executable()
		if err := os.Remove(execPath); err != nil {
			if os.IsPermission(err) {
				sudoCmd := exec.Command("sudo", "rm", execPath)
				sudoCmd.Stdin = os.Stdin
				sudoCmd.Stdout = os.Stdout
				sudoCmd.Stderr = os.Stderr
				sudoCmd.Run()
			}
		}

		fmt.Println("→ hop supprime. Zero trace.")
	},
}

func init() {
	rootCmd.AddCommand(exitCmd)
}
