package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Push la config sur le repo git",
	Run: func(cmd *cobra.Command, args []string) {
		dir := config.HopDir()

		if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
			fmt.Println("→ Initialisation du repo git...")
			runGit(dir, "init")
		}

		// Add only safe files (not secrets, keys, env files)
		runGit(dir, "add", "config.yml", ".gitignore")
		runGit(dir, "add", "install.sh")
		runGit(dir, "add", "dotfiles/")
		// Stage any deletions
		runGit(dir, "add", "-u")

		status := exec.Command("git", "-C", dir, "status", "--porcelain")
		out, _ := status.Output()
		if len(out) == 0 {
			fmt.Println("Rien à sync.")
			return
		}

		runGit(dir, "commit", "-m", "hop sync")

		remote := exec.Command("git", "-C", dir, "remote", "get-url", "origin")
		if err := remote.Run(); err == nil {
			fmt.Println("→ Push...")
			runGit(dir, "push")
		} else {
			fmt.Println("Pas de remote. Utilise: git -C ~/.hop remote add origin <url>")
		}

		fmt.Println("→ Sync terminé.")
	},
}

func runGit(dir string, args ...string) {
	fullArgs := append([]string{"-C", dir}, args...)
	sh := exec.Command("git", fullArgs...)
	sh.Stdout = os.Stdout
	sh.Stderr = os.Stderr
	sh.Run()
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
