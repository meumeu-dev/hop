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
	Short: "Supprime hop completement (binaire + config)",
	Run: func(cmd *cobra.Command, args []string) {
		hopDir := config.HopDir()
		execPath, _ := os.Executable()

		fmt.Println("Ceci va supprimer:")
		fmt.Printf("  %s (config, cles, secrets)\n", hopDir)
		fmt.Printf("  %s (binaire)\n", execPath)
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

		// Remove ~/.hop/
		if err := os.RemoveAll(hopDir); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur suppression %s: %v\n", hopDir, err)
			hasError = true
		} else {
			fmt.Printf("→ %s supprime\n", hopDir)
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
					fmt.Fprintf(os.Stderr, "Erreur suppression %s: %v\n", execPath, sudoErr)
					hasError = true
				} else {
					fmt.Printf("→ %s supprime\n", execPath)
				}
			} else {
				fmt.Fprintf(os.Stderr, "Erreur suppression %s: %v\n", execPath, err)
				hasError = true
			}
		} else {
			fmt.Printf("→ %s supprime\n", execPath)
		}

		if hasError {
			os.Exit(1)
		}
		fmt.Println("→ hop desinstalle.")
	},
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}
