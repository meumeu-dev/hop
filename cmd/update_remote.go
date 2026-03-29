package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var updateRemoteYes bool
var updateRemoteMachine string

var updateRemoteCmd = &cobra.Command{
	Use:   "push-update [machine]",
	Short: "Pousse la mise a jour hop sur les machines connectees",
	Long: `Sans argument: met a jour toutes les machines.
Avec argument: met a jour une machine specifique.

hop push-update          # toutes les machines (avec confirmation)
hop push-update rpi      # une seule machine
hop push-update -y       # toutes, sans confirmation
hop push-update -y rpi   # une seule, sans confirmation`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if len(cfg.Machines) == 0 {
			fmt.Println("Aucune machine configuree.")
			return
		}

		machines := make(map[string]config.Machine)
		if len(args) > 0 {
			name := cfg.ResolveAlias(args[0])
			m, ok := cfg.Machines[name]
			if !ok {
				fmt.Fprintf(os.Stderr, "Machine '%s' non trouvee.\n", args[0])
				os.Exit(1)
			}
			machines[name] = m
		} else {
			machines = cfg.Machines
		}

		fmt.Printf("→ Mise a jour de %d machine(s)\n\n", len(machines))

		reader := bufio.NewReader(os.Stdin)
		success := 0
		failed := 0
		skipped := 0

		for name, machine := range machines {
			if !updateRemoteYes {
				fmt.Printf("Mettre a jour '%s' (%s@%s) ? [o/N/q]: ", name, machine.User, machine.IP)
				choice, _ := reader.ReadString('\n')
				choice = strings.TrimSpace(strings.ToLower(choice))
				if choice == "q" || choice == "quit" {
					fmt.Println("Arrete.")
					break
				}
				if choice != "o" && choice != "oui" && choice != "y" && choice != "yes" {
					skipped++
					continue
				}
			}

			fmt.Printf("→ [%s] ", name)

			// Check if hop exists on remote
			target, viaTunnel := detectTarget(machine)
			checkArgs, checkTarget := buildSSHArgs(cfg, target, viaTunnel)
			checkArgs = append(checkArgs, "-o", "ConnectTimeout=10", checkTarget, "--", "which hop || command -v hop")
			checkCmd := exec.Command("ssh", checkArgs...)
			hopPath, err := checkCmd.Output()
			if err != nil || strings.TrimSpace(string(hopPath)) == "" {
				fmt.Println("hop non installe — skip")
				skipped++
				continue
			}

			remoteHop := strings.TrimSpace(string(hopPath))
			fmt.Printf("trouve (%s) — ", remoteHop)

			// Check remote version
			verArgs, verTarget := buildSSHArgs(cfg, target, viaTunnel)
			verArgs = append(verArgs, "-o", "ConnectTimeout=10", verTarget, "--", remoteHop, "version")
			verCmd := exec.Command("ssh", verArgs...)
			verOut, _ := verCmd.Output()
			remoteVer := strings.TrimSpace(string(verOut))
			if remoteVer != "" {
				fmt.Printf("v%s — ", remoteVer)
			}

			// Run update
			fmt.Println("update...")
			start := time.Now()
			updArgs, updTarget := buildSSHArgs(cfg, target, viaTunnel)
			updArgs = append(updArgs, "-o", "ConnectTimeout=10", updTarget, "--", remoteHop, "update", "-y")
			updCmd := exec.Command("ssh", updArgs...)
			updCmd.Stdout = os.Stdout
			updCmd.Stderr = os.Stderr
			if err := updCmd.Run(); err != nil {
				fmt.Printf("  ✗ [%s] echec: %v\n", name, err)
				failed++
			} else {
				elapsed := time.Since(start).Round(time.Millisecond)
				fmt.Printf("  ✓ [%s] OK (%s)\n", name, elapsed)
				success++
			}
			fmt.Println()
		}

		fmt.Printf("→ Resultat: %d OK, %d echecs, %d ignores\n", success, failed, skipped)
	},
}

func init() {
	updateRemoteCmd.Flags().BoolVarP(&updateRemoteYes, "yes", "y", false, "Pas de confirmation par machine")
	rootCmd.AddCommand(updateRemoteCmd)
}
