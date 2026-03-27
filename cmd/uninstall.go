package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Supprime hop completement (binaire + config + services)",
	Run: func(cmd *cobra.Command, args []string) {
		hopDir := config.HopDir()
		permanentDir := config.PermanentDir()
		execPath, _ := os.Executable()

		fmt.Println("Ceci va supprimer:")
		if config.IsInstalled() {
			fmt.Printf("  %s (config permanente)\n", permanentDir)
		}
		if hopDir != permanentDir {
			fmt.Printf("  %s (config sandbox)\n", hopDir)
		}
		fmt.Printf("  %s (binaire)\n", execPath)
		if config.IsInstalled() {
			fmt.Println("  Service cloudflared (si installe)")
		}
		fmt.Println()

		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Confirmer la suppression ? [oui/N]: ")
		confirm, _ := reader.ReadString('\n')
		confirm = strings.TrimSpace(strings.ToLower(confirm))

		if confirm != "oui" && confirm != "yes" {
			fmt.Println("Annule.")
			return
		}

		hasError := false

		// Stop and remove cloudflared service if installed
		if config.IsInstalled() {
			exec.Command("sudo", "cloudflared", "service", "uninstall").Run()
		}

		// Remove sandbox dir
		if err := os.RemoveAll(hopDir); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur suppression %s: %v\n", hopDir, err)
			hasError = true
		} else {
			fmt.Printf("→ %s supprime\n", hopDir)
		}

		// Remove permanent dir (if different)
		if permanentDir != hopDir {
			if _, err := os.Stat(permanentDir); err == nil {
				if err := os.RemoveAll(permanentDir); err != nil {
					fmt.Fprintf(os.Stderr, "Erreur suppression %s: %v\n", permanentDir, err)
					hasError = true
				} else {
					fmt.Printf("→ %s supprime\n", permanentDir)
				}
			}
		}

		// Remove binary
		if err := os.Remove(execPath); err != nil {
			if os.IsPermission(err) {
				fmt.Printf("→ Suppression de %s (sudo)...\n", execPath)
				sudoCmd := exec.Command("sudo", "rm", execPath)
				sudoCmd.Stdin = os.Stdin
				sudoCmd.Stdout = os.Stdout
				sudoCmd.Stderr = os.Stderr
				if sudoErr := sudoCmd.Run(); sudoErr != nil {
					fmt.Fprintf(os.Stderr, "Erreur: %v\n", sudoErr)
					hasError = true
				} else {
					fmt.Printf("→ %s supprime\n", execPath)
				}
			} else {
				fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
				hasError = true
			}
		} else {
			fmt.Printf("→ %s supprime\n", execPath)
		}

		if hasError {
			os.Exit(1)
		}
		fmt.Println("→ hop desinstalle. Zero trace.")
	},
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}
