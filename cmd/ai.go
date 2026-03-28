package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var aiEnable bool
var aiDisable bool
var aiLimits bool
var aiMCP string

var aiCmd = &cobra.Command{
	Use:   "ai <question>",
	Short: "Assistant IA qui comprend ta config hop",
	Long: `hop ai — assistant IA contextuel (necessite Cloudflare configure).

Il connait tes machines, services et aliases, et peut proposer
des commandes hop a executer. Utilise Cloudflare Workers AI (gratuit).

Examples:
  hop ai "comment me connecter a pc1 ?"
  hop ai "envoie le fichier backup.tar sur le rpi"
  hop ai --enable
  hop ai --disable
  hop ai --limits`,
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if aiMCP != "" {
			if aiMCP == "off" || aiMCP == "reset" {
				cfg.MCPEndpoint = ""
				cfg.Save()
				fmt.Println("→ MCP desactive. Workers AI sera utilise.")
			} else {
				cfg.MCPEndpoint = aiMCP
				cfg.AIEnabled = true
				cfg.Save()
				fmt.Printf("→ MCP configure: %s\n", aiMCP)
			}
			return
		}

		if aiEnable {
			runAIEnable(cfg)
			return
		}
		if aiDisable {
			runAIDisable(cfg)
			return
		}
		if aiLimits {
			runAILimits(cfg)
			return
		}

		if !cfg.AIEnabled {
			fmt.Println("hop ai — assistant IA")
			fmt.Println()
			fmt.Println("Cet assistant connait ta config (machines, services, aliases) et peut")
			fmt.Println("repondre a tes questions ou proposer des commandes hop a executer.")
			fmt.Println()
			fmt.Println("Sources IA :")
			fmt.Println("  - MCP : connecte n'importe quel LLM (Claude, GPT, etc.)")
			fmt.Println("  - Workers AI : Cloudflare gratuit (hop config)")
			fmt.Println()
			fmt.Println("Pour activer : hop ai --enable")
			fmt.Println("Pour MCP    : hop ai --mcp https://mon-endpoint/v1/chat/completions")
			fmt.Println()
			fmt.Println("Pour activer : hop ai --enable")
			return
		}

		if len(args) == 0 {
			cmd.Help()
			return
		}

		question := strings.Join(args, " ")
		runAIAsk(cfg, question)
	},
}

func runAIEnable(cfg *config.Config) {
	if cfg.AIEnabled {
		fmt.Println("L'assistant IA est deja active.")
		return
	}

	fmt.Println("=== Activation de hop ai ===")
	fmt.Println()
	fmt.Println("AVERTISSEMENT : les donnees de config (noms de machines, IPs, utilisateurs,")
	fmt.Println("noms de services, aliases) seront envoyees a Cloudflare Workers AI.")
	fmt.Println()
	fmt.Println("Les secrets (cles SSH, tokens API, mots de passe) ne sont JAMAIS envoyes.")
	fmt.Println("Necessite: Cloudflare configure (hop config).")
	fmt.Println()
	fmt.Print("Activer ? [o/N] ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	if input != "o" && input != "oui" && input != "y" && input != "yes" {
		fmt.Println("Annule.")
		return
	}

	cfg.AIEnabled = true
	if err := cfg.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur sauvegarde: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()
	fmt.Println("→ hop ai active.")
	fmt.Println()
	fmt.Println("Utilisation :")
	fmt.Println("  hop ai \"comment me connecter a pc1 ?\"")
	fmt.Println("  hop ai \"quelle commande pour le service web ?\"")
}

func runAIDisable(cfg *config.Config) {
	if !cfg.AIEnabled {
		fmt.Println("L'assistant IA est deja desactive.")
		return
	}
	cfg.AIEnabled = false
	if err := cfg.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur sauvegarde: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("→ hop ai desactive.")
}

func runAILimits(cfg *config.Config) {
	accountID, apiKey, err := loadCFCredentials(cfg)
	if err != nil || accountID == "" {
		fmt.Fprintln(os.Stderr, "CF_ACCOUNT_ID non configure.")
		fmt.Fprintln(os.Stderr, "Ajoute CF_ACCOUNT_ID=xxx dans ton fichier cloudflare.env")
		os.Exit(1)
	}
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "CF_API_KEY non configure (hop config).")
		os.Exit(1)
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/ai/usage", accountID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "Erreur API Cloudflare (HTTP %d): %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	// Pretty-print JSON
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, body, "", "  "); err == nil {
		fmt.Println(pretty.String())
	} else {
		fmt.Println(string(body))
	}
}

// safeContext builds a context string from config, stripping secrets.
func safeContext(cfg *config.Config) string {
	var sb strings.Builder

	sb.WriteString("=== Config hop ===\n")

	// Mode
	if config.IsInstalled() {
		sb.WriteString("Mode: installe (~/.hop/)\n")
	} else {
		sb.WriteString("Mode: sandbox\n")
	}

	// Machines (IP and user are safe metadata, no credentials)
	if len(cfg.Machines) > 0 {
		sb.WriteString("\nMachines:\n")
		for name, m := range cfg.Machines {
			sb.WriteString(fmt.Sprintf("  %s: ip=%s user=%s", name, m.IP, m.User))
			if m.Tunnel != "" {
				sb.WriteString(fmt.Sprintf(" tunnel=%s", m.Tunnel))
			}
			if len(m.Services) > 0 {
				var svcs []string
				for svcName := range m.Services {
					svcs = append(svcs, svcName)
				}
				sb.WriteString(fmt.Sprintf(" services=[%s]", strings.Join(svcs, ",")))
			}
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("\nMachines: (aucune)\n")
	}

	// Services (cmd is safe — it's a shell command, not a secret)
	if len(cfg.Services) > 0 {
		sb.WriteString("\nServices:\n")
		for name, svc := range cfg.Services {
			sb.WriteString(fmt.Sprintf("  %s: %s", name, svc.Desc))
			if svc.Cmd != "" {
				sb.WriteString(fmt.Sprintf(" (cmd: %s)", svc.Cmd))
			}
			if svc.Builtin {
				sb.WriteString(" [builtin]")
			}
			sb.WriteString("\n")
		}
	}

	// Aliases
	if len(cfg.Aliases) > 0 {
		sb.WriteString("\nAliases:\n")
		for alias, target := range cfg.Aliases {
			sb.WriteString(fmt.Sprintf("  %s -> %s\n", alias, target))
		}
	}

	// CF domain (hostname is safe, not the key)
	if cfg.Cloudflare.Domain != "" {
		sb.WriteString(fmt.Sprintf("\nCloudflare domain: %s\n", cfg.Cloudflare.Domain))
	}

	return sb.String()
}

// loadCFCredentials reads CF_ACCOUNT_ID and CF_API_KEY from the cloudflare.env file.
// Returns (accountID, apiKey, error). Never exposes keys to LLM.
func loadCFCredentials(cfg *config.Config) (string, string, error) {
	envPath := cfg.Cloudflare.EnvFile
	if envPath == "" {
		envPath = filepath.Join(config.HopDir(), "cloudflare.env")
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		return "", "", err
	}

	var accountID, apiKey string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "CF_ACCOUNT_ID=") {
			accountID = strings.TrimPrefix(line, "CF_ACCOUNT_ID=")
		}
		if strings.HasPrefix(line, "CF_API_KEY=") {
			apiKey = strings.TrimPrefix(line, "CF_API_KEY=")
		}
	}
	return accountID, apiKey, nil
}

const systemPrompt = `Tu es l'assistant hop. Tu connais la config de l'utilisateur. Tu peux repondre en texte ou proposer une commande hop a executer. Si tu proposes une commande, prefixe-la avec CMD: sur une ligne separee. Reponds de maniere concise et utile.`

// askWorkersAI sends a prompt to Cloudflare Workers AI and returns the response text.
func validateAccountID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if !((c >= 'a' && c <= 'f') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

func askWorkersAI(accountID, apiKey, prompt string) (string, error) {
	if !validateAccountID(accountID) {
		return "", fmt.Errorf("account ID invalide")
	}
	payload := map[string]interface{}{
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": prompt},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/ai/run/@cf/meta/llama-3.3-70b-instruct-fp8-fast", accountID)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("workers ai: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("workers ai HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Result struct {
			Response string `json:"response"`
		} `json:"result"`
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse workers ai response: %w", err)
	}
	if !result.Success {
		if len(result.Errors) > 0 {
			return "", fmt.Errorf("workers ai: %s", result.Errors[0].Message)
		}
		return "", fmt.Errorf("workers ai: requete echouee")
	}
	return result.Result.Response, nil
}

// handleResponse displays the LLM response and optionally executes a hop command.
func handleResponse(response string) {
	lines := strings.Split(strings.TrimSpace(response), "\n")
	var textLines []string
	var hopCmd string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "CMD:") {
			hopCmd = strings.TrimSpace(strings.TrimPrefix(trimmed, "CMD:"))
		} else {
			textLines = append(textLines, line)
		}
	}

	// Display text part
	text := strings.TrimSpace(strings.Join(textLines, "\n"))
	if text != "" {
		fmt.Println(text)
	}

	// Handle command proposal
	if hopCmd != "" {
		fmt.Println()
		fmt.Printf("Commande proposee : %s\n", hopCmd)
		fmt.Print("Executer ? [o/N] ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		if input == "o" || input == "oui" || input == "y" || input == "yes" {
			runHopCommand(hopCmd)
		} else {
			fmt.Println("Commande non executee.")
		}
	}
}

// safeCommands whitelist — AI cannot propose destructive commands
var safeCommands = map[string]bool{
	"ssh": true, "ping": true, "list": true,
	"send": true, "receive": true, "pair": true,
	"tunnel": true, "add": true, "alias": true,
	"dashboard": true, "version": true, "export": true,
}

func runHopCommand(cmd string) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return
	}

	if parts[0] == "hop" {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return
	}

	if !safeCommands[parts[0]] {
		fmt.Fprintf(os.Stderr, "→ Commande '%s' bloquee (non autorisee via AI).\n", parts[0])
		return
	}

	fmt.Printf("→ Execution: hop %s\n", strings.Join(parts, " "))
	rootCmd.SetArgs(parts)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
	}
}

// askMCP sends a prompt to an MCP-compatible endpoint
func askMCP(endpoint string, prompt string) (string, error) {
	// MCP uses OpenAI-compatible chat completions format
	payload := map[string]interface{}{
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": prompt},
		},
		"stream": false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("MCP: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("MCP HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	// Try OpenAI format first
	var oaiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &oaiResp); err == nil && len(oaiResp.Choices) > 0 {
		return oaiResp.Choices[0].Message.Content, nil
	}

	// Try simple format
	var simpleResp struct {
		Response string `json:"response"`
		Result   string `json:"result"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(respBody, &simpleResp); err == nil {
		if simpleResp.Response != "" {
			return simpleResp.Response, nil
		}
		if simpleResp.Result != "" {
			return simpleResp.Result, nil
		}
		if simpleResp.Content != "" {
			return simpleResp.Content, nil
		}
	}

	return string(respBody), nil
}

func runAIAsk(cfg *config.Config, question string) {
	ctx := safeContext(cfg)
	fullPrompt := fmt.Sprintf("%s\n\n%s\n\nQuestion: %s", systemPrompt, ctx, question)

	var response string
	var err error

	// Priority 1: MCP endpoint if configured
	if cfg.MCPEndpoint != "" {
		fmt.Printf("→ MCP (%s)\n\n", cfg.MCPEndpoint)
		response, err = askMCP(cfg.MCPEndpoint, fullPrompt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur MCP: %v\n", err)
			fmt.Fprintln(os.Stderr, "Fallback Workers AI...")
		}
	}

	// Priority 2: Workers AI
	if response == "" {
		accountID, apiKey, cfErr := loadCFCredentials(cfg)
		if cfErr != nil || accountID == "" || apiKey == "" {
			if cfg.MCPEndpoint == "" {
				fmt.Fprintln(os.Stderr, "Ni MCP ni Cloudflare configure (hop config).")
			} else {
				fmt.Fprintln(os.Stderr, "MCP echoue et Cloudflare non configure.")
			}
			os.Exit(1)
		}

		fmt.Println("→ Cloudflare Workers AI (llama-3.3-70b)")
		fmt.Println()
		response, err = askWorkersAI(accountID, apiKey, fullPrompt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur Workers AI: %v\n", err)
			os.Exit(1)
		}
	}

	handleResponse(response)
}

func init() {
	aiCmd.Flags().BoolVar(&aiEnable, "enable", false, "Active l'assistant IA")
	aiCmd.Flags().BoolVar(&aiDisable, "disable", false, "Desactive l'assistant IA")
	aiCmd.Flags().BoolVar(&aiLimits, "limits", false, "Affiche l'utilisation Workers AI")
	aiCmd.Flags().StringVar(&aiMCP, "mcp", "", "Configure un endpoint MCP (URL ou 'off')")
	rootCmd.AddCommand(aiCmd)
}
