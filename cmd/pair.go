package cmd

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/meumeu-dev/hop/internal/pairing"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
)

var pairMode string

var pairCmd = &cobra.Command{
	Use:   "pair [token]",
	Short: "Appaire cette machine avec un autre hop",
	Long: `Sans argument: met cette machine en attente de pairing (mode serveur).
Avec un token: se connecte à la machine en attente (mode client).

Le token est affiché par 'hop pair' sur l'autre machine.
Formats:
  Worker:  <pair_id>.<code>.<token>
  LAN:     <code> (6 chiffres)
  Worker:  utilise le relay par defaut ou configure avec hop worker url`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			runPairServer()
		} else {
			runPairClient(args[0])
		}
	},
}

func buildPairData() (string, *pairing.PairData) {
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

	hostKey := ""
	hostKeyData, err := os.ReadFile("/etc/ssh/ssh_host_ed25519_key.pub")
	if err == nil {
		hostKey = strings.TrimSpace(string(hostKeyData))
	}

	data := &pairing.PairData{
		Hostname:  hostname,
		IP:        localIP,
		IPs:       detectAllIPs(),
		User:      user,
		PublicKey: pubKey,
		HostKey:   hostKey,
		Version:   version,
	}

	return code, data
}

func runPairServer() {
	code, data := buildPairData()

	// Ask mode if not specified via flag
	if pairMode == "" || pairMode == "auto" {
		reader := bufio.NewReader(os.Stdin)
		fmt.Println("Mode de pairing:")
		fmt.Println("  1) Auto (LAN + relay en parallele)")
		fmt.Println("  2) LAN uniquement")
		fmt.Println("  3) Relay uniquement")
		fmt.Print("Choix [1]: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)
		switch choice {
		case "2", "lan":
			pairMode = "lan"
		case "3", "relay":
			pairMode = "relay"
		default:
			pairMode = "auto"
		}
		fmt.Println()
	}

	switch pairMode {
	case "lan":
		runPairServerLAN(code, data)
		return
	case "relay", "worker":
		runPairServerWorker(code, data)
		return
	}

	// Default: register on worker + broadcast LAN simultaneously
	// First who responds wins
	fmt.Println("→ Enregistrement sur le relay...")
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
	fmt.Println()
	fmt.Printf("Ou sur le meme reseau:  hop pair %s\n", code)

	if err := copyToClipboard(pairToken); err == nil {
		fmt.Println("\n(token copie dans le presse-papier)")
	}
	fmt.Println()
	fmt.Println("En attente de connexion (LAN + relay)... (expire dans 2 min)")

	// Race: LAN broadcast vs worker poll
	type pairResult struct {
		data *pairing.PairData
		via  string
	}
	resultCh := make(chan pairResult, 1)

	// Start LAN broadcast in background
	go func() {
		resp, err := pairing.StartLANServerWithTimeout(code, data, 2*time.Minute)
		if err == nil {
			resultCh <- pairResult{resp, "LAN"}
		}
	}()

	// Poll worker in background
	go func() {
		resp, err := pairing.WaitForResponse(session, 2*time.Minute)
		if err == nil {
			resultCh <- pairResult{resp, "relay"}
		}
	}()

	select {
	case result := <-resultCh:
		fmt.Printf("\n→ Connexion recue via %s\n", result.via)
		finalizePairServer(result.data, code, data)
	case <-time.After(2 * time.Minute):
		fmt.Fprintln(os.Stderr, "\nTimeout: aucune reponse recue.")
		os.Exit(1)
	}
}

func runPairServerLAN(code string, data *pairing.PairData) {
	fmt.Println("→ Mode LAN activé")
	fmt.Println()
	fmt.Println("Sur l'autre machine (même réseau), lance:")
	fmt.Printf("  hop pair %s\n", code)

	if err := copyToClipboard(code); err == nil {
		fmt.Println()
		fmt.Println("(aussi copié dans le presse-papier)")
	}
	fmt.Println()
	fmt.Println("En attente de connexion LAN... (expire dans 2 min)")

	response, err := pairing.StartLANServer(code, data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nErreur: %v\n", err)
		os.Exit(1)
	}

	finalizePairServer(response, code, data)
}

func runPairServerWorker(code string, data *pairing.PairData) {
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

	if err := copyToClipboard(pairToken); err == nil {
		fmt.Println()
		fmt.Println("(aussi copié dans le presse-papier)")
	}
	fmt.Println()
	fmt.Println("En attente de connexion... (expire dans 2 min)")

	response, err := pairing.WaitForResponse(session, 2*time.Minute)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nErreur: %v\n", err)
		os.Exit(1)
	}

	finalizePairServer(response, code, data)
}

func checkVersionMismatch(remoteVersion string) {
	if remoteVersion == "" || version == "dev" {
		return
	}
	if remoteVersion != version && remoteVersion != "v"+version && "v"+remoteVersion != version {
		fmt.Printf("\n⚠ Version differente: local %s, distant %s\n", version, remoteVersion)
		fmt.Println("  Mettez les deux machines a jour: hop update")
		fmt.Println("  (seul le binaire est mis a jour, la config et les services sont preserves)")
	}
}

func finalizePairServer(response *pairing.PairData, code string, data *pairing.PairData) {

	fmt.Println()
	checkVersionMismatch(response.Version)
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

	// Add remote machine to config — find best reachable IP
	cfg, _ := config.Load()
	if cfg != nil {
		if err := config.ValidateName(response.Hostname); err == nil {
			bestIP := response.IP
			if len(response.IPs) > 0 {
				if reachable := findReachableIP(response.IPs); reachable != "" {
					bestIP = reachable
				}
			}

			tunnel := ""
			if cfg.Cloudflare.Domain != "" {
				tunnel = response.Hostname + "." + cfg.Cloudflare.Domain
			}
			cfg.Machines[response.Hostname] = config.Machine{
				IP:       bestIP,
				User:     response.User,
				Tunnel:   tunnel,
				Services: make(map[string]config.MachineService),
			}

			// Ask for alias
			alias := askAlias(response.Hostname)
			if alias != "" {
				if cfg.Aliases == nil {
					cfg.Aliases = make(map[string]string)
				}
				cfg.Aliases[alias] = response.Hostname
				fmt.Printf("→ Alias '%s' -> '%s'\n", alias, response.Hostname)
			}

			cfg.Save()
		}
	}

	// Apply CF domain if received
	if response.CFDomain != "" {
		fmt.Printf("→ Domaine Cloudflare: %s\n", response.CFDomain)
		pairing.ApplyCFConfig(response.CFDomain)
	}

	finalName := response.Hostname
	if cfg != nil && cfg.Aliases != nil {
		for a, t := range cfg.Aliases {
			if t == response.Hostname {
				finalName = a
				break
			}
		}
	}
	fmt.Printf("\n→ Tu peux maintenant faire: hop ssh %s\n", finalName)

	// Check if the remote machine can reach us back
	checkAndOfferTunnel(response.Hostname)
}

func runPairClient(pairToken string) {
	// Detect token format:
	// - 8 chars alphanumeric → LAN mode
	// - "<pair_id>.<code>.<token>" → Worker mode

	// Check if it's a short code without dots (LAN mode)
	// LAN code: 8 alphanumeric chars, no dots or colons
	if len(pairToken) == 8 && !strings.Contains(pairToken, ".") && !strings.Contains(pairToken, ":") && isAlphanumeric(pairToken) {
		runPairClientLAN(pairToken)
		return
	}

	// Worker mode: pairID.code.token
	runPairClientWorker(pairToken)
}

func isAlphanumeric(s string) bool {
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

func buildClientResponse() (string, *pairing.PairData) {
	hostname, _ := os.Hostname()

	_, pubKey, err := pairing.EnsureSSHKey()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur génération clé SSH: %v\n", err)
		os.Exit(1)
	}

	user := os.Getenv("USER")
	localIP := detectLocalIP()
	response := &pairing.PairData{
		Hostname:  hostname,
		IP:        localIP,
		IPs:       detectAllIPs(),
		PublicKey: pubKey,
		User:      user,
		Version:   version,
	}

	cfg, _ := config.Load()
	if cfg != nil && cfg.Cloudflare.Domain != "" {
		response.CFDomain = cfg.Cloudflare.Domain
	}

	return hostname, response
}

func runPairClientLAN(code string) {
	fmt.Println("→ Mode LAN: écoute des broadcasts...")

	_, response := buildClientResponse()

	serverData, err := pairing.ConnectLAN(code, response)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
		os.Exit(1)
	}

	finalizePairClient(serverData)
}

func runPairClientWorker(pairToken string) {
	parts := strings.SplitN(pairToken, ".", 3)
	if len(parts) != 3 {
		fmt.Fprintln(os.Stderr, "Token invalide. Copie le token complet affiché par 'hop pair'.")
		os.Exit(1)
	}

	pairID := parts[0]
	code := parts[1]
	token := parts[2]

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

	_, response := buildClientResponse()

	session := &pairing.PairSession{
		PairID: pairID,
		Token:  token,
		Code:   code,
	}

	if err := pairing.SendResponse(session, response); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
		os.Exit(1)
	}

	finalizePairClient(serverData)
}

func finalizePairClient(serverData *pairing.PairData) {
	checkVersionMismatch(serverData.Version)

	if err := pairing.AddAuthorizedKey(serverData.PublicKey); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur ajout clé SSH: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil || cfg == nil {
		fmt.Fprintf(os.Stderr, "Erreur chargement config: %v\n", err)
		os.Exit(1)
	}

	// Add machine to config — find best reachable IP
	bestIP := serverData.IP
	if len(serverData.IPs) > 0 {
		if reachable := findReachableIP(serverData.IPs); reachable != "" {
			bestIP = reachable
		}
	}

	tunnel := ""
	if cfg != nil && cfg.Cloudflare.Domain != "" {
		tunnel = serverData.Hostname + "." + cfg.Cloudflare.Domain
	}

	cfg.Machines[serverData.Hostname] = config.Machine{
		IP:       bestIP,
		User:     serverData.User,
		Tunnel:   tunnel,
		Services: make(map[string]config.MachineService),
	}

	// Ask for alias
	alias := askAlias(serverData.Hostname)
	if alias != "" {
		if cfg.Aliases == nil {
			cfg.Aliases = make(map[string]string)
		}
		cfg.Aliases[alias] = serverData.Hostname
		fmt.Printf("→ Alias '%s' -> '%s'\n", alias, serverData.Hostname)
	}

	cfg.Save()

	finalName := serverData.Hostname
	if alias != "" {
		finalName = alias
	}

	fmt.Println()
	fmt.Printf("→ Pairing réussi avec '%s' !\n", serverData.Hostname)
	fmt.Printf("→ Machine ajoutée (IP: %s", serverData.IP)
	if tunnel != "" {
		fmt.Printf(", tunnel: %s", tunnel)
	}
	fmt.Println(")")

	// Transfer CF credentials via SSH and setup tunnel
	if cfg != nil && cfg.Cloudflare.Domain != "" {
		cfEmail, cfAPIKey := pairing.LoadCFCredentials()
		if cfAPIKey != "" {
			fmt.Println()
			fmt.Println("→ Transfert des identifiants Cloudflare via SSH...")
			transferAndSetupTunnel(serverData, cfg.Cloudflare.Domain, cfEmail, cfAPIKey)
		}
	}

	fmt.Printf("\n→ Tu peux maintenant faire: hop ssh %s\n", finalName)

	// Check if the remote can reach us
	checkAndOfferTunnel(serverData.Hostname)
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

	fmt.Println("  → Identifiants transférés !")
	fmt.Println()
	fmt.Printf("  → Pour finaliser, lance sur %s:\n", server.Hostname)
	fmt.Printf("    hop tunnel setup %s\n", server.Hostname)
}

func askAlias(hostname string) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("\nAlias pour '%s' (entree pour skip): ", hostname)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	if err := config.ValidateName(input); err != nil {
		fmt.Fprintf(os.Stderr, "Alias invalide: %v\n", err)
		return ""
	}
	return input
}

// checkAndOfferTunnel warns if machines are on different subnets
func checkAndOfferTunnel(remoteHostname string) {
	cfg, err := config.Load()
	if err != nil || cfg == nil {
		return
	}

	remoteMachine, ok := cfg.Machines[remoteHostname]
	if !ok || remoteMachine.Tunnel != "" {
		return
	}

	localIP := detectLocalIP()
	if localIP == "" || remoteMachine.IP == "" {
		return
	}

	localParts := strings.Split(localIP, ".")
	remoteParts := strings.Split(remoteMachine.IP, ".")
	if len(localParts) == 4 && len(remoteParts) == 4 {
		if localParts[0] == remoteParts[0] && localParts[1] == remoteParts[1] && localParts[2] == remoteParts[2] {
			return
		}
	}

	fmt.Println()
	fmt.Printf("⚠ Reseaux differents (%s vs %s)\n", localIP, remoteMachine.IP)
	fmt.Println("  Pour l'acces distant: hop tunnel setup")
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
	// Best method: get the IP of the interface that routes to LAN
	conn, err := net.DialTimeout("udp4", "192.168.0.1:80", 100*time.Millisecond)
	if err == nil {
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		conn.Close()
		if !localAddr.IP.IsLoopback() && !localAddr.IP.IsUnspecified() {
			return localAddr.IP.String()
		}
	}

	// Fallback: try common gateway
	conn, err = net.DialTimeout("udp4", "10.0.0.1:80", 100*time.Millisecond)
	if err == nil {
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		conn.Close()
		if !localAddr.IP.IsLoopback() && !localAddr.IP.IsUnspecified() {
			return localAddr.IP.String()
		}
	}

	// Last fallback: hostname -I, prefer 192.168.x.x
	out, err := exec.Command("hostname", "-I").Output()
	if err == nil {
		ips := splitSpaces(string(out))
		// First pass: prefer 192.168.x.x
		for _, ip := range ips {
			if strings.HasPrefix(ip, "192.168.") {
				return ip
			}
		}
		// Second pass: any non-loopback
		for _, ip := range ips {
			if ip != "" && ip != "127.0.0.1" {
				return ip
			}
		}
	}
	return ""
}

func detectAllIPs() []string {
	out, err := exec.Command("hostname", "-I").Output()
	if err != nil {
		return nil
	}
	raw := splitSpaces(string(out))
	var ips []string
	for _, ip := range raw {
		if ip != "" && ip != "127.0.0.1" && !strings.Contains(ip, ":") { // skip IPv6
			ips = append(ips, ip)
		}
	}
	return ips
}

// isPrivateIP checks if an IP is in a private/LAN range
func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	privateRanges := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}
	for _, cidr := range privateRanges {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// findReachableIP tests all private IPs and returns the first one reachable on SSH port
func findReachableIP(ips []string) string {
	for _, ip := range ips {
		if !isPrivateIP(ip) {
			continue
		}
		conn, err := net.DialTimeout("tcp", ip+":22", 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return ip
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

func init() {
	pairCmd.Flags().StringVarP(&pairMode, "mode", "m", "auto", "Mode de pairing: auto, lan, relay")
	rootCmd.AddCommand(pairCmd)
}
