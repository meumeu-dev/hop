package cmd

import (
	"fmt"
	"os"
	"time"

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

	// Get local IP
	localIP := ""
	cfg, err := config.Load()
	if err == nil {
		// Try to detect local IP
		for _, m := range cfg.Machines {
			if m.IP != "" {
				localIP = m.IP
				break
			}
		}
	}

	// Get current user
	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}

	// Get tunnel hostname
	tunnel := ""
	if cfg != nil && cfg.Cloudflare.Domain != "" {
		tunnel = hostname + "." + cfg.Cloudflare.Domain
	}

	// Generate code
	code := pairing.GenerateCode()

	// Build pair data
	data := &pairing.PairData{
		Hostname:  hostname,
		IP:        localIP,
		User:      user,
		PublicKey: pubKey,
		Tunnel:    tunnel,
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

	// Wait for response
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

	// Cleanup
	pairing.Cleanup(code)

	fmt.Println()
	fmt.Printf("→ Pairing réussi avec '%s' !\n", response.Hostname)
	fmt.Printf("→ Clé SSH de '%s' ajoutée à authorized_keys\n", response.Hostname)
	fmt.Println("→ Cette machine est maintenant accessible via hop.")
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

	// Send our response back
	user := os.Getenv("USER")
	response := &pairing.PairData{
		Hostname:  hostname,
		PublicKey: pubKey,
		User:      user,
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

	// Add machine to config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
		os.Exit(1)
	}

	machineName := serverData.Hostname
	cfg.Machines[machineName] = config.Machine{
		IP:       serverData.IP,
		User:     serverData.User,
		Tunnel:   serverData.Tunnel,
		Services: make(map[string]config.MachineService),
	}

	if err := cfg.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Printf("→ Pairing réussi avec '%s' !\n", serverData.Hostname)
	fmt.Printf("→ Machine ajoutée à ta config\n")
	fmt.Printf("→ Tu peux maintenant faire: hop ssh %s\n", machineName)
}

func init() {
	rootCmd.AddCommand(pairCmd)
}
