package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Nucleaire: supprime TOUT (config + binaire + services + cloudflared)",
	Run: func(cmd *cobra.Command, args []string) {
		hopDir := config.HopDir()
		permanentDir := config.PermanentDir()
		execPath, _ := os.Executable()
		home, _ := os.UserHomeDir()
		cloudflaredDir := filepath.Join(home, ".cloudflared")

		fmt.Println("Ceci va TOUT supprimer:")
		fmt.Printf("  %s (config sandbox)\n", hopDir)
		if permanentDir != hopDir {
			fmt.Printf("  %s (config permanente)\n", permanentDir)
		}
		fmt.Printf("  %s (cloudflared)\n", cloudflaredDir)
		fmt.Printf("  %s (binaire)\n", execPath)
		fmt.Println("  Service cloudflared (si installe)")
		fmt.Println()

		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Confirmer la suppression TOTALE ? [oui/N]: ")
		confirm, _ := reader.ReadString('\n')
		confirm = strings.TrimSpace(strings.ToLower(confirm))

		if confirm != "oui" && confirm != "yes" {
			fmt.Println("Annule.")
			return
		}

		// Stop and remove cloudflared service (Linux/macOS only)
		if runtime.GOOS != "windows" {
			exec.Command("sudo", "cloudflared", "service", "uninstall").Run()
			exec.Command("sudo", "systemctl", "stop", "cloudflared").Run()
		}

		// Remove sandbox dir
		os.RemoveAll(hopDir)
		fmt.Printf("→ %s supprime\n", hopDir)

		// Remove permanent dir
		if permanentDir != hopDir {
			os.RemoveAll(permanentDir)
			fmt.Printf("→ %s supprime\n", permanentDir)
		}

		// Remove cloudflared config
		os.RemoveAll(cloudflaredDir)
		fmt.Printf("→ %s supprime\n", cloudflaredDir)

		// Remove cloudflared binary if in ~/.hop/bin/
		cfBinName := "cloudflared"
		if runtime.GOOS == "windows" {
			cfBinName = "cloudflared.exe"
		}
		hopBin := filepath.Join(permanentDir, "bin", cfBinName)
		os.Remove(hopBin)
		sandboxBin := filepath.Join(hopDir, "bin", cfBinName)
		os.Remove(sandboxBin)

		// Remove hop binary
		if err := os.Remove(execPath); err != nil {
			if os.IsPermission(err) && runtime.GOOS != "windows" {
				sudoCmd := exec.Command("sudo", "rm", execPath)
				sudoCmd.Stdin = os.Stdin
				sudoCmd.Stdout = os.Stdout
				sudoCmd.Stderr = os.Stderr
				sudoCmd.Run()
			}
		}
		fmt.Printf("→ %s supprime\n", execPath)

		fmt.Println("\n→ hop desinstalle. Zero trace.")
	},
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}
