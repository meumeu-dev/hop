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
	"github.com/meumeu-dev/hop/internal/tunnel"
	"github.com/spf13/cobra"
)

var tunnelCmd = &cobra.Command{
	Use:   "tunnel",
	Short: "Gère les tunnels Cloudflare",
}

var tunnelSetupCmd = &cobra.Command{
	Use:   "setup [nom]",
	Short: "Configure un tunnel Cloudflare sur cette machine",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if cfg.Cloudflare.EnvFile != "" {
			loadEnvFile(config.ExpandPath(cfg.Cloudflare.EnvFile))
		}

		// Auto-install cloudflared
		cfPath, err := cf.EnsureInstalled()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		reader := bufio.NewReader(os.Stdin)

		// Determine tunnel name
		tunnelName := ""
		if len(args) > 0 {
			tunnelName = args[0]
		} else {
			hostname, _ := os.Hostname()
			fmt.Printf("Nom du tunnel [%s]: ", hostname)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			if input == "" {
				tunnelName = hostname
			} else {
				tunnelName = input
			}
		}

		// Step 1: Login if needed
		fmt.Println("\n→ Étape 1: Authentification Cloudflare")
		certPath := os.ExpandEnv("$HOME/.cloudflared/cert.pem")
		if _, err := os.Stat(certPath); os.IsNotExist(err) {
			if err := cf.Run("tunnel", "login"); err != nil {
				fmt.Fprintf(os.Stderr, "Erreur login: %v\n", err)
				os.Exit(1)
			}
		} else {
			fmt.Println("  Déjà authentifié.")
		}

		// Step 2: Create tunnel
		fmt.Printf("\n→ Étape 2: Création du tunnel '%s'\n", tunnelName)
		createCmd := exec.Command(cfPath, "tunnel", "create", tunnelName)
		createCmd.Stdout = os.Stdout
		createCmd.Stderr = os.Stderr
		if err := createCmd.Run(); err != nil {
			fmt.Println("  Le tunnel existe peut-être déjà, on continue...")
		}

		// Step 3: Route DNS
		if cfg.Cloudflare.Domain != "" {
			hostname := tunnelName + "." + cfg.Cloudflare.Domain
			fmt.Printf("\n→ Étape 3: Route DNS %s\n", hostname)
			routeCmd := exec.Command(cfPath, "tunnel", "route", "dns", tunnelName, hostname)
			routeCmd.Stdout = os.Stdout
			routeCmd.Stderr = os.Stderr
			if err := routeCmd.Run(); err != nil {
				fmt.Printf("  Route DNS peut-être déjà existante pour %s\n", hostname)
			}
		} else {
			fmt.Println("\n→ Étape 3: Pas de domaine configuré, route DNS ignorée.")
			fmt.Println("  Configure ton domaine avec 'hop init' ou 'hop dashboard'.")
		}

		// Step 4: Generate config
		fmt.Println("\n→ Étape 4: Génération de la config cloudflared")
		cfConfigDir := os.ExpandEnv("$HOME/.cloudflared")
		cfConfigPath := cfConfigDir + "/config.yml"

		// Find tunnel UUID
		listCmd := exec.Command(cfPath, "tunnel", "list", "-o", "json")
		listOut, err := listCmd.Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: impossible de lister les tunnels\n")
			os.Exit(1)
		}

		// Simple extraction of tunnel ID (avoid importing encoding/json just for this)
		tunnelID := ""
		lines := strings.Split(string(listOut), "\"")
		for i, part := range lines {
			if part == "id" && i+2 < len(lines) {
				tunnelID = lines[i+2]
				break
			}
		}

		if tunnelID != "" {
			cfConfig := fmt.Sprintf(`tunnel: %s
credentials-file: %s/%s.json

ingress:
  - hostname: %s.%s
    service: ssh://localhost:22
  - service: http_status:404
`, tunnelID, cfConfigDir, tunnelID, tunnelName, cfg.Cloudflare.Domain)

			if err := os.WriteFile(cfConfigPath, []byte(cfConfig), 0600); err != nil {
				fmt.Fprintf(os.Stderr, "Erreur écriture config: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("  Config écrite: %s\n", cfConfigPath)
		}

		// Step 5: Ask to install as service
		fmt.Print("\n→ Installer cloudflared comme service systemd ? [o/N]: ")
		confirm, _ := reader.ReadString('\n')
		confirm = strings.TrimSpace(strings.ToLower(confirm))
		if confirm == "o" || confirm == "oui" || confirm == "y" || confirm == "yes" {
			serviceCmd := exec.Command("sudo", cfPath, "service", "install")
			serviceCmd.Stdin = os.Stdin
			serviceCmd.Stdout = os.Stdout
			serviceCmd.Stderr = os.Stderr
			if err := serviceCmd.Run(); err != nil {
				fmt.Println("  Erreur installation service. Tu peux lancer manuellement:")
				fmt.Printf("  sudo %s service install\n", cfPath)
			} else {
				fmt.Println("  Service installé et démarré.")
			}
		} else {
			fmt.Println("  Pour lancer manuellement:")
			fmt.Printf("  %s tunnel run %s\n", cfPath, tunnelName)
		}

		fmt.Println("\n→ Tunnel configuré !")
	},
}

var tunnelStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Affiche l'état des tunnels",
	Run: func(cmd *cobra.Command, args []string) {
		if !cf.IsInstalled() {
			fmt.Fprintln(os.Stderr, "cloudflared non installé. Lance: hop tunnel setup")
			os.Exit(1)
		}
		if err := cf.Run("tunnel", "list"); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
	},
}

// allowedEnvPrefixes limits which env vars can be set from the env file
var allowedEnvPrefixes = []string{"CF_", "CLOUDFLARE_", "TUNNEL_"}

func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		allowed := false
		for _, prefix := range allowedEnvPrefixes {
			if strings.HasPrefix(strings.ToUpper(key), prefix) {
				allowed = true
				break
			}
		}
		if allowed {
			os.Setenv(key, strings.TrimSpace(parts[1]))
		}
	}
}

var tunnelProvider string

var tunnelQuickCmd = &cobra.Command{
	Use:   "quick",
	Short: "Lance un tunnel temporaire (zero config, plusieurs providers)",
	Run: func(cmd *cobra.Command, args []string) {
		maxRetries := 3
		for retry := 0; retry < maxRetries; retry++ {
			if tunnelProvider == "" {
				tunnelProvider = askTunnelProvider()
			}

			var err error
			switch tunnelProvider {
			case "bore":
				err = runTunnelBore()
			case "cloudflare":
				runTunnelCFPermanent()
				return
			case "worker":
				runTunnelWorkerSetup()
				return
			default:
				fmt.Fprintf(os.Stderr, "Provider inconnu: %s\n", tunnelProvider)
				os.Exit(1)
			}

			if err == nil {
				return // tunnel ran and exited normally
			}

			fmt.Fprintf(os.Stderr, "\n→ Tunnel echoue: %v\n\n", err)
			if retry < maxRetries-1 {
				fmt.Println("Essayer un autre provider ?")
			} else {
				fmt.Fprintln(os.Stderr, "Nombre maximum de tentatives atteint.")
				os.Exit(1)
			}
			tunnelProvider = "" // reset to show menu again
		}
	},
}

func askTunnelProvider() string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Provider de tunnel:")
	fmt.Println("  1) bore.pub      (auto-install, TCP direct)")
	fmt.Println("  2) Cloudflare    (tunnel permanent, necessite compte CF)")
	fmt.Println("  3) Worker perso  (configurer ton propre relay)")
	fmt.Print("Choix [1]: ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)
	fmt.Println()
	switch choice {
	case "2", "cloudflare", "cf":
		return "cloudflare"
	case "3", "worker":
		return "worker"
	default:
		return "bore"
	}
}

func registerAndKeepAlive(tunnelHost string) {
	if err := tunnel.Register(tunnelHost); err != nil {
		fmt.Printf("→ Warning: enregistrement worker echoue: %v\n", err)
	} else {
		fmt.Println("→ Enregistre sur le worker (resolv auto pour les autres machines)")
	}

	fmt.Println()
	fmt.Println("Le tunnel reste actif tant que ce processus tourne.")
	fmt.Println("Ctrl+C pour arreter.")
	fmt.Println()

	go func() {
		for {
			<-time.After(30 * time.Minute)
			tunnel.Register(tunnelHost)
		}
	}()
}


// knownTunnelDomains are suffixes that indicate a real tunnel URL (not docs/marketing)
var knownTunnelDomains = []string{
	".bore.pub",
	"bore.pub:",
}

func checkAndRegisterTunnelURL(line string, provider string) {
	for _, word := range strings.Fields(line) {
		word = strings.Trim(word, ".,;\"'`()[]{}<>|")

		// Strip scheme
		host := word
		host = strings.TrimPrefix(host, "https://")
		host = strings.TrimPrefix(host, "http://")
		host = strings.TrimPrefix(host, "tcp://")
		host = strings.TrimRight(host, "/")

		if host == "" {
			continue
		}

		// Only register if it matches a known tunnel domain
		for _, suffix := range knownTunnelDomains {
			if strings.Contains(host, suffix) {
				fmt.Printf("\n→ Tunnel detecte: %s\n", host)
				tunnel.Register(host)
				return
			}
		}
	}
}

func runTunnelBore() error {
	borePath := findOrInstallBore()
	if borePath == "" {
		return fmt.Errorf("impossible d'installer bore")
	}

	fmt.Println("→ Lancement du tunnel via bore.pub...")

	tunnelCmd := exec.Command(borePath, "local", "22", "--to", "bore.pub")
	tunnelCmd.Stdin = os.Stdin

	stderrPipe, _ := tunnelCmd.StderrPipe()
	stdoutPipe, _ := tunnelCmd.StdoutPipe()

	if err := tunnelCmd.Start(); err != nil {
		return fmt.Errorf("bore: %w", err)
	}

	go func() {
		s := bufio.NewScanner(stderrPipe)
		for s.Scan() {
			line := s.Text()
			fmt.Fprintln(os.Stderr, line)
			if strings.Contains(line, "bore.pub:") {
				for _, word := range strings.Fields(line) {
					if strings.Contains(word, "bore.pub:") {
						host := strings.TrimRight(word, ".,;")
						fmt.Printf("\n→ Tunnel actif: %s\n", host)
						tunnel.Register(host)
					}
				}
			}
		}
	}()

	go func() {
		s := bufio.NewScanner(stdoutPipe)
		for s.Scan() {
			fmt.Println(s.Text())
		}
	}()

	fmt.Println("  Ctrl+C pour arreter.")
	if err := tunnelCmd.Wait(); err != nil {
		return fmt.Errorf("bore: %w", err)
	}
	return nil
}

func findOrInstallBore() string {
	// Check if bore is already installed
	if p, err := exec.LookPath("bore"); err == nil {
		return p
	}

	binDir := config.HopDir() + "/bin"
	borePath := binDir + "/bore"
	if _, err := os.Stat(borePath); err == nil {
		return borePath
	}

	// Auto-install
	fmt.Println("→ Installation de bore...")
	os.MkdirAll(binDir, 0700)

	arch := "x86_64"
	switch runtime.GOARCH {
	case "arm64":
		arch = "aarch64"
	case "arm":
		arch = "armv7"
	}

	// Get latest version
	verOut, err := exec.Command("curl", "-sSf", "https://api.github.com/repos/ekzhang/bore/releases/latest").Output()
	boreVersion := "v0.6.0"
	if err == nil {
		for _, line := range strings.Split(string(verOut), "\n") {
			if strings.Contains(line, "tag_name") {
				parts := strings.Split(line, "\"")
				for i, p := range parts {
					if p == "tag_name" && i+2 < len(parts) {
						boreVersion = parts[i+2]
						break
					}
				}
			}
		}
	}

	url := fmt.Sprintf("https://github.com/ekzhang/bore/releases/download/%s/bore-%s-%s-unknown-linux-musl.tar.gz", boreVersion, boreVersion, arch)
	fmt.Printf("→ Telechargement bore %s...\n", boreVersion)

	tmpFile := "/tmp/bore.tar.gz"
	dlCmd := exec.Command("curl", "-sSfL", "-o", tmpFile, url)
	if err := dlCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur telechargement bore: %v\n", err)
		return ""
	}

	// Extract — bore binary may be at root or in a subdirectory
	extractCmd := exec.Command("tar", "-xzf", tmpFile, "-C", "/tmp")
	if err := extractCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur extraction bore: %v\n", err)
		return ""
	}
	// Find the bore binary
	exec.Command("bash", "-c", fmt.Sprintf("find /tmp -name 'bore' -type f -executable 2>/dev/null | head -1 | xargs -I{} mv {} %s", borePath)).Run()
	if _, err := os.Stat(borePath); err != nil {
		// Try direct move
		exec.Command("mv", "/tmp/bore", borePath).Run()
	}
	os.Remove(tmpFile)
	os.Chmod(borePath, 0755)

	fmt.Println("→ bore installe dans ~/.hop/bin/")
	return borePath
}

func runTunnelCFPermanent() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
		os.Exit(1)
	}

	if cfg.Cloudflare.Domain == "" {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Domaine Cloudflare (ex: mondomaine.dev): ")
		domain, _ := reader.ReadString('\n')
		domain = strings.TrimSpace(domain)
		if domain == "" {
			fmt.Fprintln(os.Stderr, "Domaine requis pour un tunnel permanent.")
			fmt.Fprintln(os.Stderr, "Pas de domaine ? Utilise trycloudflare (option 1) a la place.")
			os.Exit(1)
		}
		cfg.Cloudflare.Domain = domain
		cfg.Save()
	}

	// Delegate to tunnel setup
	fmt.Println("→ Configuration du tunnel Cloudflare permanent...")
	hopBin, _ := os.Executable()
	setupCmd := exec.Command(hopBin, "tunnel", "setup")
	setupCmd.Stdout = os.Stdout
	setupCmd.Stderr = os.Stderr
	setupCmd.Stdin = os.Stdin
	setupCmd.Run()
}

func runTunnelWorkerSetup() {
	fmt.Println("→ Configuration du worker perso")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  1) Deployer un worker sur ton compte Cloudflare (gratuit)")
	fmt.Println("  2) Configurer l'URL d'un worker/relay existant")

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Choix [1]: ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	if choice == "2" {
		fmt.Print("URL du worker (https://...): ")
		url, _ := reader.ReadString('\n')
		url = strings.TrimSpace(url)
		if url == "" || !strings.HasPrefix(url, "https://") {
			fmt.Fprintln(os.Stderr, "URL invalide (doit commencer par https://)")
			os.Exit(1)
		}
		cfg, _ := config.Load()
		if cfg != nil {
			cfg.WorkerURL = url
			cfg.Save()
			fmt.Printf("→ Worker configure: %s\n", url)
		}
		return
	}

	// Deploy worker
	hopBin, _ := os.Executable()
	deployCmd := exec.Command(hopBin, "worker", "deploy")
	deployCmd.Stdout = os.Stdout
	deployCmd.Stderr = os.Stderr
	deployCmd.Stdin = os.Stdin
	deployCmd.Run()
}

func init() {
	tunnelQuickCmd.Flags().StringVarP(&tunnelProvider, "provider", "p", "", "Provider: trycloudflare, localhost.run, serveo.net, bore, cloudflare, worker")
	tunnelCmd.AddCommand(tunnelSetupCmd)
	tunnelCmd.AddCommand(tunnelStatusCmd)
	tunnelCmd.AddCommand(tunnelQuickCmd)
	rootCmd.AddCommand(tunnelCmd)
}
