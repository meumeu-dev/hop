package cmd

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/meumeu-dev/hop/internal/cloudflared"
	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var sendToFlag string
var receiveToFlag string


var sendCmd = &cobra.Command{
	Use:   "send <machine> <file-dir-or-url>",
	Short: "Envoie un fichier, dossier ou URL vers une machine distante",
	Long: `Envoie un fichier local ou telecharge une URL directement sur la machine distante.

hop send rpi fichier.txt              # fichier local
hop send rpi dossier/                 # dossier entier
hop send rpi https://example.com/file # URL (telecharge directement sur la machine)
hop send rpi fichier.txt --to /opt/   # destination custom`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		machineName := args[0]
		src := args[1]

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

		// Detect if source is a URL
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			runSendURL(src, machineName, machine)
			return
		}

		runSendFile(src, machineName, machine)
	},
}

func formatSize(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func runSendURL(url string, machineName string, machine config.Machine) {
	target, viaTunnel := detectTarget(machine)

	dest := sendToFlag
	if dest == "" {
		dest = "hop-received/"
	}

	// Shell-escape both URL and dest to prevent injection
	safeDest := shellEscape(dest)
	safeURL := shellEscape(url)

	remoteCmd := fmt.Sprintf("mkdir -p %s && cd %s && (curl -sSLO %s || wget -q %s)",
		safeDest, safeDest, safeURL, safeURL)

	fmt.Printf("→ Telechargement de %s sur %s\n", url, machineName)

	sshCmdArgs, cleanTarget := buildSSHArgs(target, viaTunnel)
	sshCmdArgs = append(sshCmdArgs, cleanTarget, "--", remoteCmd)

	sh := exec.Command("ssh", sshCmdArgs...)
	sh.Stdin = os.Stdin
	sh.Stdout = os.Stdout
	sh.Stderr = os.Stderr
	if err := sh.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("→ Telecharge sur %s dans %s\n", machineName, dest)
}

func runSendFile(src string, machineName string, machine config.Machine) {
	info, err := os.Stat(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Source introuvable: %v\n", err)
		os.Exit(1)
	}

	target, viaTunnel := detectTarget(machine)

	dest := sendToFlag
	if dest == "" {
		dest = "hop-received/"
	}

	// Check remote disk space BEFORE anything
	fileSize := info.Size()
	checkArgs, checkTarget := buildSSHArgs(target, viaTunnel)
	checkArgs = append(checkArgs, checkTarget, "--", "df", "--output=avail", "-B1", shellEscape(dest), "2>/dev/null", "|", "tail", "-1")
	if out, err := exec.Command("ssh", checkArgs...).Output(); err == nil {
		avail := strings.TrimSpace(string(out))
		if availBytes, err := strconv.ParseInt(avail, 10, 64); err == nil {
			if availBytes < fileSize {
				fmt.Fprintf(os.Stderr, "Erreur: pas assez d'espace sur %s (%s dispo, %s requis)\n",
					machineName, formatSize(availBytes), formatSize(fileSize))
				os.Exit(1)
			}
			if availBytes < fileSize*2 {
				fmt.Printf("⚠ Espace limite sur %s (%s dispo pour %s)\n",
					machineName, formatSize(availBytes), formatSize(fileSize))
			}
		}
	}

	// Create remote dir after space check passed
	mkdirArgs, mkdirTarget := buildSSHArgs(target, viaTunnel)
	mkdirArgs = append(mkdirArgs, mkdirTarget, "--", "mkdir", "-p", shellEscape(dest))
	mkdirCmd := exec.Command("ssh", mkdirArgs...)
	mkdirCmd.Stderr = os.Stderr
	_ = mkdirCmd.Run()

	scpBaseArgs, scpTarget := buildSCPArgs(viaTunnel, target)
	remoteTarget := fmt.Sprintf("%s:%s", scpTarget, dest)
	if info.IsDir() {
		scpBaseArgs = append([]string{"-r"}, scpBaseArgs...)
	}
	scpBaseArgs = append(scpBaseArgs, src, remoteTarget)

	fmt.Printf("→ Envoi de '%s' (%s) vers %s:%s\n", src, formatSize(fileSize), machineName, dest)

	start := time.Now()
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
	elapsed := time.Since(start)
	speed := float64(fileSize) / elapsed.Seconds() / 1024 / 1024
	fmt.Printf("→ Transfert termine en %s (%.1f MB/s)\n", elapsed.Round(time.Millisecond), speed)

	// Integrity check: compare local and remote checksums
	if !info.IsDir() {
		fmt.Print("→ Verification integrite...")
		localHash := localMD5(src)
		if localHash != "" {
			remotePath := dest + filepath.Base(src)
			remoteHashArgs, rhTarget := buildSSHArgs(target, viaTunnel)
			remoteHashArgs = append(remoteHashArgs, rhTarget, "--", "md5sum", shellEscape(remotePath))
			out, err := exec.Command("ssh", remoteHashArgs...).Output()
			if err == nil {
				remoteHash := strings.Fields(string(out))
				if len(remoteHash) > 0 && remoteHash[0] == localHash {
					fmt.Println(" OK (MD5 identique)")
				} else {
					fmt.Println(" ERREUR (checksums differents !)")
					fmt.Fprintf(os.Stderr, "  Local:  %s\n  Distant: %s\n", localHash, string(out))
				}
			} else {
				fmt.Println(" (verification impossible)")
			}
		}
	}
}

func localMD5(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// splitHostPort splits user@host:port into (user@host, port) for quick tunnels
func splitTargetPort(target string) (string, string) {
	// Find the @ to separate user from host:port
	atIdx := strings.LastIndex(target, "@")
	if atIdx < 0 {
		return target, ""
	}
	hostPart := target[atIdx+1:]
	// Check for host:port
	if colonIdx := strings.LastIndex(hostPart, ":"); colonIdx > 0 {
		host := hostPart[:colonIdx]
		port := hostPart[colonIdx+1:]
		// Verify it's a port number
		if port != "" && port != "22" {
			return target[:atIdx+1] + host, port
		}
	}
	return target, ""
}

// buildSSHArgs builds SSH args with hop key + tunnel proxy + port if needed
// Returns (args, cleanTarget) where cleanTarget has port stripped
func buildSSHArgs(target string, viaTunnel bool) ([]string, string) {
	hopKeyPath := filepath.Join(config.HopDir(), "keys", "hop_ed25519")
	args := []string{"-i", hopKeyPath, "-o", "IdentitiesOnly=yes", "-o", "StrictHostKeyChecking=accept-new"}
	if viaTunnel {
		cfPath := cloudflared.Path()
		args = append(args, "-o", fmt.Sprintf("ProxyCommand=%s access ssh --hostname %%h", cfPath))
	}
	cleanTarget := target
	if ct, port := splitTargetPort(target); port != "" {
		args = append(args, "-p", port)
		cleanTarget = ct
	}
	return args, cleanTarget
}

func buildSCPArgs(viaTunnel bool, target string) ([]string, string) {
	hopKeyPath := filepath.Join(config.HopDir(), "keys", "hop_ed25519")
	args := []string{"-i", hopKeyPath, "-o", "IdentitiesOnly=yes", "-o", "StrictHostKeyChecking=accept-new"}
	if viaTunnel {
		cfPath := cloudflared.Path()
		args = append(args, "-o", fmt.Sprintf("ProxyCommand=%s access ssh --hostname %%h", cfPath))
	}
	cleanTarget := target
	if ct, port := splitTargetPort(target); port != "" {
		args = append(args, "-P", port)
		cleanTarget = ct
	}
	return args, cleanTarget
}

var receiveCmd = &cobra.Command{
	Use:   "receive <machine> <remote-path>",
	Short: "Recoit un fichier depuis une machine distante",
	Long: `hop receive rpi /var/log/syslog         # recoit dans le dossier courant
hop receive rpi /opt/data/ --to ~/tmp  # destination custom`,
	Args: cobra.ExactArgs(2),
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

		scpBaseArgs, scpTarget := buildSCPArgs(viaTunnel, target)
		src := fmt.Sprintf("%s:%s", scpTarget, remotePath)
		scpBaseArgs = append([]string{"-r"}, scpBaseArgs...)
		scpBaseArgs = append(scpBaseArgs, src, dest)

		fmt.Printf("→ Reception de '%s' depuis %s vers '%s'\n", remotePath, machineName, dest)

		start := time.Now()
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
		elapsed := time.Since(start)
		fmt.Printf("→ Transfert termine en %s\n", elapsed.Round(time.Millisecond))
	},
}

func init() {
	sendCmd.Flags().StringVar(&sendToFlag, "to", "", "Chemin de destination sur la machine distante (defaut: ~/hop-received/)")
	receiveCmd.Flags().StringVar(&receiveToFlag, "to", "", "Chemin de destination local (defaut: repertoire courant)")
	rootCmd.AddCommand(sendCmd)
	rootCmd.AddCommand(receiveCmd)
}
