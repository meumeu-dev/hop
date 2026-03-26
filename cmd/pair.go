package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	cf "github.com/meumeu-dev/hop/internal/cloudflared"
	"github.com/meumeu-dev/hop/internal/config"
	"github.com/meumeu-dev/hop/internal/pairing"
	"github.com/spf13/cobra"
)

var pairCmd = &cobra.Command{
	Use:   "pair [code]",
	Short: "Appaire cette machine avec un autre hop",
	Long: `Sans argument: met cette machine en attente de pairing (mode serveur).
Avec un code: se connecte à la machine en attente (mode client).`,
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

	// Ensure SSH key exists
	_, pubKey, err := pairing.EnsureSSHKey()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur génération clé SSH: %v\n", err)
		os.Exit(1)
	}

	// Detect local IP
	localIP := detectLocalIP()

	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}

	// Generate code
	code := pairing.GenerateCode()

	// Build pair data (server doesn't send CF config, it receives it)
	data := &pairing.PairData{
		Hostname:  hostname,
		IP:        localIP,
		User:      user,
		PublicKey: pubKey,
	}

	// Publish encrypted data
	fmt.Println("→ Enregistrement sur le serveur de pairing...")
	if err := pairing.PublishPairData(code, data); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("╔══════════════════════════════╗")
	fmt.Printf("║  Code de pairing: %s     ║\n", code)
	fmt.Println("╠══════════════════════════════╣")
	fmt.Println("║  Sur l'autre PC, tape:       ║")
	fmt.Printf("║  hop pair %s              ║\n", code)
	fmt.Println("╚══════════════════════════════╝")
	fmt.Println()
	fmt.Println("En attente de connexion... (expire dans 5 min)")

	// Wait for response from PC principal
	response, err := pairing.WaitForResponse(code, 5*time.Minute)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nErreur: %v\n", err)
		pairing.Cleanup(code)
		os.Exit(1)
	}

	// Add the PC's public key to authorized_keys
	if err := pairing.AddAuthorizedKey(response.PublicKey); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur ajout clé SSH: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Printf("→ Pairing réussi avec '%s' !\n", response.Hostname)
	fmt.Printf("→ Clé SSH ajoutée\n")

	// Apply CF config if received
	if response.CFDomain != "" {
		fmt.Println()
		fmt.Println("→ Configuration Cloudflare reçue...")
		if err := pairing.ApplyCFConfig(response.CFDomain, response.CFEmail, response.CFAPIKey); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur config CF: %v\n", err)
		} else {
			fmt.Printf("→ Domaine: %s\n", response.CFDomain)

			// Auto tunnel setup
			fmt.Println()
			fmt.Println("→ Configuration du tunnel Cloudflare...")
			setupTunnelAuto(hostname, response.CFDomain, response.CFEmail, response.CFAPIKey)
		}
	}

	pairing.Cleanup(code)
	fmt.Println()
	fmt.Println("→ Cette machine est prête !")
}

func runPairClient(code string) {
	hostname, _ := os.Hostname()

	// Ensure SSH key exists
	_, pubKey, err := pairing.EnsureSSHKey()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur génération clé SSH: %v\n", err)
		os.Exit(1)
	}

	// Fetch and decrypt the server's data
	fmt.Println("→ Récupération des données de pairing...")
	serverData, err := pairing.FetchPairData(code)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("→ Machine trouvée: %s (%s@%s)\n", serverData.Hostname, serverData.User, serverData.IP)

	// Load CF credentials to send to the server
	cfg, _ := config.Load()
	cfEmail, cfAPIKey := pairing.LoadCFCredentials()

	// Build response with SSH key + CF config
	user := os.Getenv("USER")
	response := &pairing.PairData{
		Hostname: hostname,
		PublicKey: pubKey,
		User:     user,
	}

	// Include CF config if available
	if cfg != nil && cfg.Cloudflare.Domain != "" && cfAPIKey != "" {
		response.CFDomain = cfg.Cloudflare.Domain
		response.CFEmail = cfEmail
		response.CFAPIKey = cfAPIKey
		fmt.Printf("→ Envoi config Cloudflare (%s)\n", cfg.Cloudflare.Domain)
	}

	if err := pairing.SendResponse(code, response); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur envoi réponse: %v\n", err)
		os.Exit(1)
	}

	// Add the server's public key to authorized_keys
	if err := pairing.AddAuthorizedKey(serverData.PublicKey); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur ajout clé SSH: %v\n", err)
		os.Exit(1)
	}

	// Add machine to config with auto tunnel hostname
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

	if err := cfg.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Printf("→ Pairing réussi avec '%s' !\n", serverData.Hostname)
	fmt.Printf("→ Machine ajoutée (IP: %s", serverData.IP)
	if tunnel != "" {
		fmt.Printf(", tunnel: %s", tunnel)
	}
	fmt.Println(")")
	fmt.Printf("→ Tu peux maintenant faire: hop ssh %s\n", serverData.Hostname)
}

func detectLocalIP() string {
	// Try to find the local IP by dialing a public address (no actual connection)
	conn, err := exec.Command("hostname", "-I").Output()
	if err == nil {
		parts := fmt.Sprintf("%s", conn)
		ips := fmt.Sprintf("%s", parts)
		if len(ips) > 0 {
			// Take first IP
			for _, ip := range splitSpaces(ips) {
				if ip != "" && ip != "127.0.0.1" {
					return ip
				}
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

	// Set env for cloudflared
	os.Setenv("CLOUDFLARE_API_KEY", cfAPIKey)
	os.Setenv("CLOUDFLARE_EMAIL", cfEmail)

	// Login
	fmt.Println("  → Authentification Cloudflare...")
	loginCmd := exec.Command(cfPath, "tunnel", "login")
	loginCmd.Stdin = os.Stdin
	loginCmd.Stdout = os.Stdout
	loginCmd.Stderr = os.Stderr
	if err := loginCmd.Run(); err != nil {
		fmt.Println("  → Login échoué, tu peux le faire plus tard avec: hop tunnel setup")
		return
	}

	// Create tunnel
	tunnelName := hostname
	fmt.Printf("  → Création du tunnel '%s'...\n", tunnelName)
	createCmd := exec.Command(cfPath, "tunnel", "create", tunnelName)
	createCmd.Stdout = os.Stdout
	createCmd.Stderr = os.Stderr
	createCmd.Run() // ignore error if already exists

	// Route DNS
	tunnelHostname := hostname + "." + cfDomain
	fmt.Printf("  → Route DNS %s...\n", tunnelHostname)
	routeCmd := exec.Command(cfPath, "tunnel", "route", "dns", tunnelName, tunnelHostname)
	routeCmd.Stdout = os.Stdout
	routeCmd.Stderr = os.Stderr
	routeCmd.Run()

	// Generate config
	cfConfigDir := os.ExpandEnv("$HOME/.cloudflared")
	cfConfigPath := cfConfigDir + "/config.yml"

	listCmd := exec.Command(cfPath, "tunnel", "list", "-o", "json")
	listOut, err := listCmd.Output()
	if err == nil {
		// Extract tunnel ID
		tunnelID := ""
		lines := fmt.Sprintf("%s", listOut)
		parts := splitByQuotes(lines)
		for i, part := range parts {
			if part == "id" && i+2 < len(parts) {
				tunnelID = parts[i+2]
				break
			}
		}

		if tunnelID != "" {
			cfConfig := fmt.Sprintf("tunnel: %s\ncredentials-file: %s/%s.json\n\ningress:\n  - hostname: %s\n    service: ssh://localhost:22\n  - service: http_status:404\n",
				tunnelID, cfConfigDir, tunnelID, tunnelHostname)
			os.WriteFile(cfConfigPath, []byte(cfConfig), 0600)
			fmt.Printf("  → Config écrite: %s\n", cfConfigPath)
		}
	}

	// Install as service
	fmt.Println("  → Installation du service systemd...")
	serviceCmd := exec.Command("sudo", cfPath, "service", "install")
	serviceCmd.Stdin = os.Stdin
	serviceCmd.Stdout = os.Stdout
	serviceCmd.Stderr = os.Stderr
	if err := serviceCmd.Run(); err != nil {
		fmt.Printf("  → Service non installé. Lance manuellement: %s tunnel run %s\n", cfPath, tunnelName)
	} else {
		fmt.Println("  → Tunnel actif !")
	}
}

func splitByQuotes(s string) []string {
	var result []string
	current := ""
	inQuote := false
	for _, c := range s {
		if c == '"' {
			if inQuote {
				result = append(result, current)
				current = ""
			}
			inQuote = !inQuote
		} else if inQuote {
			current += string(c)
		}
	}
	return result
}

func init() {
	rootCmd.AddCommand(pairCmd)
}
