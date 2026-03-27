package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/meumeu-dev/hop/internal/cloudflared"
	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var sendToFlag string
var receiveToFlag string

// scpArgs builds the scp arguments for a given target, using the hop key and
// optionally the cloudflared ProxyCommand for CF tunnel targets.
func scpArgs(viaTunnel bool) []string {
	hopKeyPath := filepath.Join(config.HopDir(), "keys", "hop_ed25519")
	args := []string{"-i", hopKeyPath}
	if viaTunnel {
		cfPath := cloudflared.Path()
		args = append(args, "-o", fmt.Sprintf("ProxyCommand=%s access ssh --hostname %%h", cfPath))
	}
	// Pass through progress and accept new host keys
	args = append(args, "-o", "StrictHostKeyChecking=accept-new")
	return args
}

var sendCmd = &cobra.Command{
	Use:   "send <file-or-dir> <machine>",
	Short: "Envoie un fichier ou dossier vers une machine distante",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		src := args[0]
		machineName := args[1]

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		machineName = cfg.ResolveAlias(machineName)
		machine, ok := cfg.Machines[machineName]
		if !ok {
			fmt.Fprintf(os.Stderr, "Machine '%s' non trouvee. Utilise 'hop list' pour voir les machines.\n", machineName)
			os.Exit(1)
		}

		// Check source exists
		info, err := os.Stat(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Source introuvable: %v\n", err)
			os.Exit(1)
		}

		target, viaTunnel := detectTarget(machine)

		dest := sendToFlag
		if dest == "" {
			dest = "~/hop-received/"
		}

		remoteTarget := fmt.Sprintf("%s:%s", target, dest)

		// Ensure remote destination directory exists (best-effort)
		hopKeyPath := filepath.Join(config.HopDir(), "keys", "hop_ed25519")
		mkdirArgs := []string{"-i", hopKeyPath}
		if viaTunnel {
			cfPath := cloudflared.Path()
			mkdirArgs = append(mkdirArgs, "-o", fmt.Sprintf("ProxyCommand=%s access ssh --hostname %%h", cfPath))
		}
		mkdirArgs = append(mkdirArgs, "-o", "StrictHostKeyChecking=accept-new", target, "--", "mkdir", "-p", dest)
		mkdirCmd := exec.Command("ssh", mkdirArgs...)
		mkdirCmd.Stderr = os.Stderr
		_ = mkdirCmd.Run() // best-effort

		// Build scp command
		scpBaseArgs := scpArgs(viaTunnel)
		if info.IsDir() {
			scpBaseArgs = append([]string{"-r"}, scpBaseArgs...)
		}
		scpBaseArgs = append(scpBaseArgs, src, remoteTarget)

		fmt.Printf("→ Envoi de '%s' vers %s:%s\n", src, machineName, dest)

		sh := exec.Command("scp", scpBaseArgs...)
		sh.Stdin = os.Stdin
		sh.Stdout = os.Stdout
		sh.Stderr = os.Stderr
		if err := sh.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				os.Exit(exitErr.ExitCode())
			}
			fmt.Fprintf(os.Stderr, "Erreur scp: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("→ Transfert termine.\n")
	},
}

var receiveCmd = &cobra.Command{
	Use:   "receive <machine> <remote-path>",
	Short: "Recoit un fichier depuis une machine distante",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		machineName := args[0]
		remotePath := args[1]

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		machineName = cfg.ResolveAlias(machineName)
		machine, ok := cfg.Machines[machineName]
		if !ok {
			fmt.Fprintf(os.Stderr, "Machine '%s' non trouvee. Utilise 'hop list' pour voir les machines.\n", machineName)
			os.Exit(1)
		}

		target, viaTunnel := detectTarget(machine)

		dest := receiveToFlag
		if dest == "" {
			dest = "."
		}

		src := fmt.Sprintf("%s:%s", target, remotePath)

		scpBaseArgs := scpArgs(viaTunnel)
		// Use -r to support directories transparently
		scpBaseArgs = append([]string{"-r"}, scpBaseArgs...)
		scpBaseArgs = append(scpBaseArgs, src, dest)

		fmt.Printf("→ Reception de '%s' depuis %s vers '%s'\n", remotePath, machineName, dest)

		sh := exec.Command("scp", scpBaseArgs...)
		sh.Stdin = os.Stdin
		sh.Stdout = os.Stdout
		sh.Stderr = os.Stderr
		if err := sh.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				os.Exit(exitErr.ExitCode())
			}
			fmt.Fprintf(os.Stderr, "Erreur scp: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("→ Transfert termine.\n")
	},
}

func init() {
	sendCmd.Flags().StringVar(&sendToFlag, "to", "", "Chemin de destination sur la machine distante (defaut: ~/hop-received/)")
	receiveCmd.Flags().StringVar(&receiveToFlag, "to", "", "Chemin de destination local (defaut: repertoire courant)")
	rootCmd.AddCommand(sendCmd)
	rootCmd.AddCommand(receiveCmd)
}
