package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	cf "github.com/meumeu-dev/hop/internal/cloudflared"
	"github.com/meumeu-dev/hop/internal/config"
	"github.com/meumeu-dev/hop/internal/pairing"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
)

var pairCmd = &cobra.Command{
	Use:   "pair [token]",
	Short: "Appaire cette machine avec un autre hop",
	Long: `Sans argument: met cette machine en attente de pairing (mode serveur).
Avec un token: se connecte à la machine en attente (mode client).

Le token est affiché par 'hop pair' sur l'autre machine.
Format: <pair_id>.<code>.<token>`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			runPairServer()
		} else {
			runPairClient(args[0])
		}
	},
}

func runPairServer() {
	hostname, _ := os.Hostname()

	if err := config.ValidateName(hostname); err != nil {
		fmt.Fprintf(os.Stderr, "Hostname invalide: %v\n", err)
		os.Exit(1)
	}

	_, pubKey, err := pairing.EnsureSSHKey()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur génération clé SSH: %v\n", err)
		os.Exit(1)
	}

	localIP := detectLocalIP()
	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}

	code := pairing.GenerateCode()

	// Read host key for SSH pinning
	hostKey := ""
	hostKeyData, err := os.ReadFile("/etc/ssh/ssh_host_ed25519_key.pub")
	if err == nil {
		hostKey = strings.TrimSpace(string(hostKeyData))
	}

	data := &pairing.PairData{
		Hostname:  hostname,
		IP:        localIP,
		User:      user,
		PublicKey: pubKey,
		HostKey:   hostKey,
	}

	fmt.Println("→ Enregistrement sur le serveur de pairing...")
	session, err := pairing.PublishPairData(code, data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
		os.Exit(1)
	}
	defer pairing.Cleanup(session)

	pairToken := session.PairID + "." + code + "." + session.Token

	fmt.Println()
	fmt.Println("Sur l'autre machine, lance:")
	fmt.Printf("  hop pair %s\n", pairToken)

	// Try to copy to clipboard too
	if err := copyToClipboard(pairToken); err == nil {
		fmt.Println()
		fmt.Println("(aussi copie dans le presse-papier)")
	}
	fmt.Println()
	fmt.Println("En attente de connexion... (expire dans 2 min)")

	response, err := pairing.WaitForResponse(session, 2*time.Minute)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nErreur: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Printf("→ Machine distante: %s\n", response.Hostname)
	if parsedKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(response.PublicKey)); err == nil {
		fmt.Printf("→ Empreinte SSH: %s\n", ssh.FingerprintSHA256(parsedKey))
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\nAccepter ce pairing ? [o/N]: ")
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(strings.ToLower(confirm))
	if confirm != "o" && confirm != "oui" && confirm != "y" && confirm != "yes" {
		fmt.Println("Pairing annulé.")
		os.Exit(0)
	}

	if err := pairing.AddAuthorizedKey(response.PublicKey); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur ajout clé SSH: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("→ Pairing réussi avec '%s' !\n", response.Hostname)
	fmt.Println("→ Clé SSH ajoutée")

	// Add remote machine to config
	cfg, _ := config.Load()
	if cfg != nil {
		if err := config.ValidateName(response.Hostname); err == nil {
			tunnel := ""
			if cfg.Cloudflare.Domain != "" {
				tunnel = response.Hostname + "." + cfg.Cloudflare.Domain
			}
			cfg.Machines[response.Hostname] = config.Machine{
				IP:       response.IP,
				User:     response.User,
				Tunnel:   tunnel,
				Services: make(map[string]config.MachineService),
			}
			cfg.Save()
		}
	}

	// Apply CF domain if received
	if response.CFDomain != "" {
		fmt.Printf("→ Domaine Cloudflare: %s\n", response.CFDomain)
		pairing.ApplyCFConfig(response.CFDomain)
	}

	fmt.Printf("\n→ Tu peux maintenant faire: hop ssh %s\n", response.Hostname)
}

func runPairClient(pairToken string) {
	// Parse: pairID.code.token
	parts := strings.SplitN(pairToken, ".", 3)
	if len(parts) != 3 {
		fmt.Fprintln(os.Stderr, "Token invalide. Copie le token complet affiché par 'hop pair'.")
		os.Exit(1)
	}

	pairID := parts[0]
	code := parts[1]
	token := parts[2]

	hostname, _ := os.Hostname()

	_, pubKey, err := pairing.EnsureSSHKey()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur génération clé SSH: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("→ Récupération des données de pairing...")
	serverData, err := pairing.FetchPairData(pairID, code)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
		os.Exit(1)
	}

	if err := config.ValidateName(serverData.Hostname); err != nil {
		fmt.Fprintf(os.Stderr, "Hostname distant invalide: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("→ Machine trouvée: %s (%s@%s)\n", serverData.Hostname, serverData.User, serverData.IP)
	if parsedKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(serverData.PublicKey)); err == nil {
		fmt.Printf("→ Empreinte SSH: %s\n", ssh.FingerprintSHA256(parsedKey))
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\nAccepter ce pairing ? [o/N]: ")
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(strings.ToLower(confirm))
	if confirm != "o" && confirm != "oui" && confirm != "y" && confirm != "yes" {
		fmt.Println("Pairing annulé.")
		os.Exit(0)
	}

	cfg, _ := config.Load()

	user := os.Getenv("USER")
	localIP := detectLocalIP()
	response := &pairing.PairData{
		Hostname:  hostname,
		IP:        localIP,
		PublicKey: pubKey,
		User:      user,
	}
	if cfg != nil && cfg.Cloudflare.Domain != "" {
		response.CFDomain = cfg.Cloudflare.Domain
	}

	session := &pairing.PairSession{
		PairID: pairID,
		Token:  token,
		Code:   code,
	}

	if err := pairing.SendResponse(session, response); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
		os.Exit(1)
	}

	if err := pairing.AddAuthorizedKey(serverData.PublicKey); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur ajout clé SSH: %v\n", err)
		os.Exit(1)
	}

	// Add machine to config
	tunnel := ""
	if cfg != nil && cfg.Cloudflare.Domain != "" {
		tunnel = serverData.Hostname + "." + cfg.Cloudflare.Domain
	}

	cfg.Machines[serverData.Hostname] = config.Machine{
		IP:       serverData.IP,
		User:     serverData.User,
		Tunnel:   tunnel,
		Services: make(map[string]config.MachineService),
	}
	cfg.Save()

	fmt.Println()
	fmt.Printf("→ Pairing réussi avec '%s' !\n", serverData.Hostname)
	fmt.Printf("→ Machine ajoutée (IP: %s", serverData.IP)
	if tunnel != "" {
		fmt.Printf(", tunnel: %s", tunnel)
	}
	fmt.Println(")")

	// Transfer CF credentials via SSH and setup tunnel
	if cfg.Cloudflare.Domain != "" {
		cfEmail, cfAPIKey := pairing.LoadCFCredentials()
		if cfAPIKey != "" {
			fmt.Println()
			fmt.Println("→ Transfert des identifiants Cloudflare via SSH...")
			transferAndSetupTunnel(serverData, cfg.Cloudflare.Domain, cfEmail, cfAPIKey)
		}
	}

	fmt.Printf("\n→ Tu peux maintenant faire: hop ssh %s\n", serverData.Hostname)
}

func transferAndSetupTunnel(server *pairing.PairData, cfDomain, cfEmail, cfAPIKey string) {
	target := server.User + "@" + server.IP
	hopKeyPath := config.HopDir() + "/keys/hop_ed25519"

	envContent := pairing.BuildCFEnvContent(cfEmail, cfAPIKey, cfDomain)

	fmt.Printf("  → Envoi vers %s...\n", target)

	sshArgs := []string{"-i", hopKeyPath}

	// Pin host key if available from pairing
	if server.HostKey != "" {
		knownHostsPath := config.HopDir() + "/known_hosts_tmp"
		knownEntry := fmt.Sprintf("%s %s\n", server.IP, server.HostKey)
		os.WriteFile(knownHostsPath, []byte(knownEntry), 0600)
		defer os.Remove(knownHostsPath)
		sshArgs = append(sshArgs, "-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile="+knownHostsPath)
	} else {
		sshArgs = append(sshArgs, "-o", "StrictHostKeyChecking=accept-new")
	}

	sshArgs = append(sshArgs, target, "--",
		"bash", "-c", "mkdir -p ~/.hop && cat > ~/.hop/cloudflare.env && chmod 600 ~/.hop/cloudflare.env")

	sshCmd := exec.Command("ssh", sshArgs...)
	sshCmd.Stdin = strings.NewReader(envContent)
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr
	if err := sshCmd.Run(); err != nil {
		fmt.Printf("  → Erreur SSH: %v\n", err)
		fmt.Println("  → Tu peux transférer manuellement avec: hop tunnel setup")
		return
	}

	// Update remote hop config to point to the env file
	configCmd := fmt.Sprintf("hop add service ssh --cmd ssh 2>/dev/null; true")
	_ = configCmd

	fmt.Println("  → Identifiants transférés !")
	fmt.Println()
	fmt.Printf("  → Pour finaliser, lance sur %s:\n", server.Hostname)
	fmt.Printf("    hop tunnel setup %s\n", server.Hostname)
}

func copyToClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xclip", "-selection", "clipboard")
	case "darwin":
		cmd = exec.Command("pbcopy")
	default:
		return fmt.Errorf("unsupported")
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func detectLocalIP() string {
	conn, err := exec.Command("hostname", "-I").Output()
	if err == nil {
		ips := splitSpaces(string(conn))
		for _, ip := range ips {
			if ip != "" && ip != "127.0.0.1" {
				return ip
			}
		}
	}
	return ""
}

func splitSpaces(s string) []string {
	var result []string
	current := ""
	for _, c := range s {
		if c == ' ' || c == '\n' || c == '\t' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func setupTunnelAuto(hostname, cfDomain, cfEmail, cfAPIKey string) {
	cfPath, err := cf.EnsureInstalled()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur installation cloudflared: %v\n", err)
		return
	}

	os.Setenv("CLOUDFLARE_API_KEY", cfAPIKey)
	os.Setenv("CLOUDFLARE_EMAIL", cfEmail)

	fmt.Println("  → Authentification Cloudflare...")
	loginCmd := exec.Command(cfPath, "tunnel", "login")
	loginCmd.Stdin = os.Stdin
	loginCmd.Stdout = os.Stdout
	loginCmd.Stderr = os.Stderr
	if err := loginCmd.Run(); err != nil {
		fmt.Println("  → Login échoué, lance: hop tunnel setup")
		return
	}

	tunnelName := hostname
	fmt.Printf("  → Création du tunnel '%s'...\n", tunnelName)
	exec.Command(cfPath, "tunnel", "create", "--", tunnelName).Run()

	tunnelHostname := hostname + "." + cfDomain
	fmt.Printf("  → Route DNS %s...\n", tunnelHostname)
	exec.Command(cfPath, "tunnel", "route", "dns", "--", tunnelName, tunnelHostname).Run()

	cfConfigDir := os.ExpandEnv("$HOME/.cloudflared")
	cfConfigPath := cfConfigDir + "/config.yml"

	listOut, err := exec.Command(cfPath, "tunnel", "list", "-o", "json").Output()
	if err == nil {
		tunnelID := extractTunnelID(string(listOut))
		if tunnelID != "" {
			cfConfig := fmt.Sprintf("tunnel: %s\ncredentials-file: %s/%s.json\n\ningress:\n  - hostname: %s\n    service: ssh://localhost:22\n  - service: http_status:404\n",
				tunnelID, cfConfigDir, tunnelID, tunnelHostname)
			os.WriteFile(cfConfigPath, []byte(cfConfig), 0600)
			fmt.Printf("  → Config: %s\n", cfConfigPath)
		}
	}

	fmt.Println("  → Installation service systemd...")
	serviceCmd := exec.Command("sudo", cfPath, "service", "install")
	serviceCmd.Stdin = os.Stdin
	serviceCmd.Stdout = os.Stdout
	serviceCmd.Stderr = os.Stderr
	if err := serviceCmd.Run(); err != nil {
		fmt.Printf("  → Lance manuellement: %s tunnel run %s\n", cfPath, tunnelName)
	} else {
		fmt.Println("  → Tunnel actif !")
	}
}

func extractTunnelID(jsonOutput string) string {
	parts := strings.Split(jsonOutput, "\"")
	for i, part := range parts {
		if part == "id" && i+2 < len(parts) {
			return parts[i+2]
		}
	}
	return ""
}

func init() {
	rootCmd.AddCommand(pairCmd)
}
