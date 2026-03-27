package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

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
		binRemoved := false
		if err := os.Remove(execPath); err != nil {
			if os.IsPermission(err) && runtime.GOOS != "windows" {
				sudoCmd := exec.Command("sudo", "rm", execPath)
				sudoCmd.Stdin = os.Stdin
				sudoCmd.Stdout = os.Stdout
				sudoCmd.Stderr = os.Stderr
				if sudoCmd.Run() == nil {
					binRemoved = true
				}
			}
		} else {
			binRemoved = true
		}

		if binRemoved {
			fmt.Println("→ hop supprime. Zero trace.")
		} else {
			fmt.Println("→ Config supprimee. Binaire encore present.")
			fmt.Fprintf(os.Stderr, "  sudo rm %s pour finir\n", execPath)
		}
	},
}

func init() {
	rootCmd.AddCommand(exitCmd)
}
