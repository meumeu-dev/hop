package dashboard

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/meumeu-dev/hop/internal/cloudflared"
	"github.com/meumeu-dev/hop/internal/config"
	"github.com/meumeu-dev/hop/internal/pairing"
)

//go:embed static/*
var staticFiles embed.FS

// DashboardVersion is set from cmd package
var DashboardVersion = "dev"

var configMu sync.Mutex

// Active pairing session for dashboard-initiated pairing
var activePairMu sync.Mutex
var activePairSession *pairing.PairSession
var activePairResult *pairing.PairData

type machineReq struct {
	Name   string `json:"name"`
	IP     string `json:"ip"`
	User   string `json:"user"`
	Tunnel string `json:"tunnel"`
}

type serviceReq struct {
	Name string `json:"name"`
	Cmd  string `json:"cmd"`
	Desc string `json:"desc"`
}

func basicAuthMiddleware(password string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pass, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(pass), []byte(password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="hop dashboard"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isAllowedOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()
	// Always allow localhost
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	// Allow private IPs (LAN access)
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsPrivate()
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func jsonOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// dashboardAuthMiddleware generates a per-session CSRF token for the dashboard
func dashboardAuthMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'")

		if strings.HasPrefix(r.URL.Path, "/api/") && r.Method != "GET" {
			// Require CSRF token for mutations
			csrfToken := r.Header.Get("X-Hop-CSRF")
			if subtle.ConstantTimeCompare([]byte(csrfToken), []byte(token)) != 1 {
				jsonError(w, "csrf", 403)
				return
			}
		}

		// CSRF token endpoint — only allow from localhost origins
		if r.URL.Path == "/api/csrf" {
			origin := r.Header.Get("Origin")
			referer := r.Header.Get("Referer")
			if origin == "" && referer == "" {
				jsonError(w, "forbidden", 403)
				return
			}
			if origin != "" && !isAllowedOrigin(origin) {
				jsonError(w, "forbidden", 403)
				return
			}
			if origin == "" && referer != "" && !isAllowedOrigin(referer) {
				jsonError(w, "forbidden", 403)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"token": token})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func limitBody(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
}

func registerAPIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/ping", handlePing)
	mux.HandleFunc("/api/config", handleConfig)
	mux.HandleFunc("/api/machines", handleMachines)
	mux.HandleFunc("/api/machines/versions", handleMachineVersions)
	mux.HandleFunc("/api/machines/update", handleMachineUpdate)
	mux.HandleFunc("/api/machines/", handleMachineDelete)
	mux.HandleFunc("/api/services", handleServices)
	mux.HandleFunc("/api/services/", handleServiceDelete)
	mux.HandleFunc("/api/cloudflare", handleCloudflare)
	mux.HandleFunc("/api/pair", handlePair)
	mux.HandleFunc("/api/pair/start", handlePairStart)
	mux.HandleFunc("/api/pair/status", handlePairStatus)
	mux.HandleFunc("/api/ai", handleAI)
}

func StartWithBind(port int, bind string, password string, open bool) error {
	mux := http.NewServeMux()
	registerAPIRoutes(mux)

	staticFS, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	// Generate per-session CSRF token for dashboard
	tokenBytes := make([]byte, 32)
	rand.Read(tokenBytes)
	csrfToken := hex.EncodeToString(tokenBytes)

	addr := fmt.Sprintf("%s:%d", bind, port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("port %d deja utilise: %w", port, err)
	}

	if bind == "0.0.0.0" {
		fmt.Printf("-> Dashboard sur http://%s:%d (reseau)\n", getLocalIP(), port)
		fmt.Println("   ⚠ Accessible depuis le reseau local")
	} else {
		fmt.Printf("-> Dashboard sur http://localhost:%d\n", port)
	}

	if open && bind == "127.0.0.1" {
		openBrowser(fmt.Sprintf("http://localhost:%d", port))
	}

	var handler http.Handler
	handler = dashboardAuthMiddleware(csrfToken, mux)
	if password != "" {
		handler = basicAuthMiddleware(password, handler)
	}
	server := &http.Server{
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 180 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return server.Serve(listener)
}

func getLocalIP() string {
	conn, err := net.Dial("udp4", "192.168.0.1:80")
	if err == nil {
		defer conn.Close()
		return conn.LocalAddr().(*net.UDPAddr).IP.String()
	}
	return "0.0.0.0"
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	}
	if cmd != nil {
		cmd.Start()
	}
}

// --- Handlers ---

func handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": DashboardVersion})
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	configMu.Lock()
	cfg, err := config.Load()
	if err != nil {
		configMu.Unlock()
		jsonError(w, "internal", 500)
		return
	}

	// Strip secrets — build safe copy while locked
	safeCfg := *cfg
	safeCfg.Cloudflare = config.CloudflareConfig{Domain: cfg.Cloudflare.Domain}
	// Strip Cmd from services (don't leak commands to remote API consumers)
	safeServices := make(map[string]config.Service)
	for k, s := range cfg.Services {
		safeServices[k] = config.Service{Desc: s.Desc, Builtin: s.Builtin}
	}
	safeCfg.Services = safeServices
	// Strip Cmd from machine services too
	safeMachines := make(map[string]config.Machine)
	for k, m := range cfg.Machines {
		safeServices := make(map[string]config.MachineService)
		for sk, sv := range m.Services {
			safeServices[sk] = config.MachineService{ID: sv.ID}
		}
		safeMachines[k] = config.Machine{
			IP: m.IP, User: m.User, Tunnel: m.Tunnel, Services: safeServices,
		}
	}
	safeCfg.Machines = safeMachines
	// Strip unlock credentials (service token + SSH key path) — only the
	// machine name and tunnel hostname are safe to expose.
	if len(cfg.Unlock) > 0 {
		safeUnlock := make([]config.UnlockTarget, 0, len(cfg.Unlock))
		for _, u := range cfg.Unlock {
			safeUnlock = append(safeUnlock, config.UnlockTarget{
				Name:     u.Name,
				Hostname: u.Hostname,
			})
		}
		safeCfg.Unlock = safeUnlock
	}
	configMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(safeCfg)
}

func handleMachines(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "method not allowed", 405)
		return
	}
	limitBody(w, r)

	var req machineReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "bad request", 400)
		return
	}

	if err := config.ValidateName(req.Name); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if err := config.ValidateIP(req.IP); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if err := config.ValidateUser(req.User); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if err := config.ValidateTunnel(req.Tunnel); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}

	configMu.Lock()
	defer configMu.Unlock()

	cfg, err := config.Load()
	if err != nil {
		jsonError(w, "internal", 500)
		return
	}

	existing, ok := cfg.Machines[req.Name]
	services := make(map[string]config.MachineService)
	if ok {
		services = existing.Services
	}

	cfg.Machines[req.Name] = config.Machine{
		IP:       req.IP,
		User:     req.User,
		Tunnel:   req.Tunnel,
		Services: services,
	}

	if err := cfg.Save(); err != nil {
		jsonError(w, "internal", 500)
		return
	}

	jsonOK(w)
}

func handleMachineDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		jsonError(w, "method not allowed", 405)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/machines/")
	if err := config.ValidateName(name); err != nil {
		jsonError(w, "invalid name", 400)
		return
	}

	configMu.Lock()
	defer configMu.Unlock()

	cfg, err := config.Load()
	if err != nil {
		jsonError(w, "internal", 500)
		return
	}

	delete(cfg.Machines, name)

	if err := cfg.Save(); err != nil {
		jsonError(w, "internal", 500)
		return
	}

	jsonOK(w)
}

// --- SSH helpers for version check (mirrors cmd/root.go logic) ---

func dashDetectTarget(m config.Machine) (target string, viaTunnel bool) {
	// Try LAN first
	if m.IP != "" {
		conn, err := net.DialTimeout("tcp", m.IP+":22", 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return m.User + "@" + m.IP, false
		}
	}
	// Try tunnel
	if m.Tunnel != "" {
		host, port, err := net.SplitHostPort(m.Tunnel)
		if err == nil && host != "" && port != "" && port != "22" {
			// Quick tunnel: host:port (Pinggy)
			return m.User + "@" + host + ":" + port, false
		}
		// Cloudflare tunnel
		return m.User + "@" + m.Tunnel, true
	}
	return "", false
}

func dashBuildSSHArgs(cfg *config.Config, target string, viaTunnel bool) ([]string, string) {
	hopKeyPath := filepath.Join(config.HopDir(), "keys", "hop_ed25519")
	args := []string{"-i", hopKeyPath, "-o", "IdentitiesOnly=yes", "-o", "StrictHostKeyChecking=accept-new"}
	if viaTunnel {
		cfPath := cloudflared.Path()
		proxyCmd := fmt.Sprintf("%s access ssh --hostname %%h", cfPath)
		if cfg != nil && cfg.Cloudflare.CFServiceTokenID != "" && cfg.Cloudflare.CFServiceTokenSecret != "" {
			proxyCmd += fmt.Sprintf(" --service-token-id %s --service-token-secret %s",
				cfg.Cloudflare.CFServiceTokenID, cfg.Cloudflare.CFServiceTokenSecret)
		}
		args = append(args, "-o", "ProxyCommand="+proxyCmd)
	}
	cleanTarget := target
	// Split off port if present (user@host:port)
	if atIdx := strings.LastIndex(target, "@"); atIdx >= 0 {
		hostPart := target[atIdx+1:]
		if host, port, err := net.SplitHostPort(hostPart); err == nil {
			args = append(args, "-p", port)
			cleanTarget = target[:atIdx+1] + host
		}
	}
	return args, cleanTarget
}

// --- Version check handler ---

type versionResult struct {
	Version string `json:"version"`
	Error   string `json:"error,omitempty"`
}

func handleMachineVersions(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		jsonError(w, "method not allowed", 405)
		return
	}

	configMu.Lock()
	cfg, err := config.Load()
	configMu.Unlock()
	if err != nil {
		jsonError(w, "internal", 500)
		return
	}

	if len(cfg.Machines) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{})
		return
	}

	results := make(map[string]versionResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for name, machine := range cfg.Machines {
		wg.Add(1)
		go func(name string, machine config.Machine) {
			defer wg.Done()

			target, viaTunnel := dashDetectTarget(machine)
			if target == "" {
				mu.Lock()
				results[name] = versionResult{Error: "injoignable"}
				mu.Unlock()
				return
			}

			sshArgs, sshTarget := dashBuildSSHArgs(cfg, target, viaTunnel)
			sshArgs = append(sshArgs, "-o", "ConnectTimeout=10", sshTarget, "--", "hop version 2>/dev/null || echo unknown")

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
			out, err := cmd.Output()
			ver := strings.TrimSpace(string(out))

			mu.Lock()
			if err != nil || ver == "" {
				results[name] = versionResult{Error: "erreur SSH"}
			} else if ver == "unknown" {
				results[name] = versionResult{Error: "hop non installe"}
			} else {
				results[name] = versionResult{Version: ver}
			}
			mu.Unlock()
		}(name, machine)
	}

	wg.Wait()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// --- Update handler ---

type updateReq struct {
	Name string `json:"name"`
}

type updateResult struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

func handleMachineUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "method not allowed", 405)
		return
	}
	limitBody(w, r)

	var req updateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "bad request", 400)
		return
	}
	if req.Name == "" {
		jsonError(w, "name requis", 400)
		return
	}

	// Find the hop binary
	hopBin, err := os.Executable()
	if err != nil {
		jsonError(w, "impossible de trouver le binaire hop", 500)
		return
	}

	configMu.Lock()
	cfg, err := config.Load()
	configMu.Unlock()
	if err != nil {
		jsonError(w, "internal", 500)
		return
	}

	// Determine which machines to update
	var machineNames []string
	if req.Name == "all" {
		for name := range cfg.Machines {
			machineNames = append(machineNames, name)
		}
	} else {
		resolved := cfg.ResolveAlias(req.Name)
		if _, ok := cfg.Machines[resolved]; !ok {
			jsonError(w, "machine '"+req.Name+"' non trouvee", 404)
			return
		}
		machineNames = []string{resolved}
	}

	if len(machineNames) == 0 {
		jsonError(w, "aucune machine configuree", 400)
		return
	}

	// Increase write timeout for long-running updates
	w.Header().Set("Content-Type", "application/json")

	results := make(map[string]updateResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, name := range machineNames {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, hopBin, "push-update", "-y", name)
			out, err := cmd.CombinedOutput()

			mu.Lock()
			if err != nil {
				results[name] = updateResult{
					Output: string(out),
					Error:  err.Error(),
				}
			} else {
				results[name] = updateResult{Output: string(out)}
			}
			mu.Unlock()
		}(name)
	}

	wg.Wait()

	json.NewEncoder(w).Encode(results)
}

func handleServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "method not allowed", 405)
		return
	}
	limitBody(w, r)

	var req serviceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "bad request", 400)
		return
	}

	if err := config.ValidateName(req.Name); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}

	configMu.Lock()
	defer configMu.Unlock()

	cfg, err := config.Load()
	if err != nil {
		jsonError(w, "internal", 500)
		return
	}

	if svc, ok := cfg.Services[req.Name]; ok && svc.Builtin {
		jsonError(w, "cannot modify builtin service", 400)
		return
	}

	cfg.Services[req.Name] = config.Service{
		Desc: req.Desc,
		Cmd:  req.Cmd,
	}

	if err := cfg.Save(); err != nil {
		jsonError(w, "internal", 500)
		return
	}

	jsonOK(w)
}

func handleServiceDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		jsonError(w, "method not allowed", 405)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/services/")
	if err := config.ValidateName(name); err != nil {
		jsonError(w, "invalid name", 400)
		return
	}

	configMu.Lock()
	defer configMu.Unlock()

	cfg, err := config.Load()
	if err != nil {
		jsonError(w, "internal", 500)
		return
	}

	if svc, ok := cfg.Services[name]; ok && svc.Builtin {
		jsonError(w, "cannot delete builtin service", 400)
		return
	}

	delete(cfg.Services, name)

	if err := cfg.Save(); err != nil {
		jsonError(w, "internal", 500)
		return
	}

	jsonOK(w)
}

type cloudflareReq struct {
	Domain string `json:"domain"`
	Email  string `json:"email"`
	APIKey string `json:"api_key"`
}

func handleCloudflare(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		configMu.Lock()
		cfg, err := config.Load()
		configMu.Unlock()
		if err != nil {
			jsonError(w, "internal", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"domain": cfg.Cloudflare.Domain,
		})
		return
	}

	if r.Method != "POST" {
		jsonError(w, "method not allowed", 405)
		return
	}
	limitBody(w, r)

	var req cloudflareReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "bad request", 400)
		return
	}

	configMu.Lock()
	defer configMu.Unlock()

	cfg, err := config.Load()
	if err != nil {
		jsonError(w, "internal", 500)
		return
	}

	// Write env file — strip newlines to prevent injection
	sanitize := func(s string) string {
		s = strings.ReplaceAll(s, "\n", "")
		s = strings.ReplaceAll(s, "\r", "")
		return s
	}
	envPath := filepath.Join(config.HopDir(), "cloudflare.env")
	envContent := fmt.Sprintf("CF_USER=%s\nCF_DOMAIN=%s\nCF_API_KEY=%s\n", sanitize(req.Email), sanitize(req.Domain), sanitize(req.APIKey))
	if err := os.WriteFile(envPath, []byte(envContent), 0600); err != nil {
		jsonError(w, "cannot write env file", 500)
		return
	}

	cfg.Cloudflare = config.CloudflareConfig{
		Domain:  req.Domain,
		EnvFile: envPath,
	}

	if err := cfg.Save(); err != nil {
		jsonError(w, "internal", 500)
		return
	}

	jsonOK(w)
}

// --- AI handler ---

type aiReq struct {
	Question string `json:"question"`
}

type aiResp struct {
	Answer  string `json:"answer"`
	Command string `json:"command,omitempty"`
	Source  string `json:"source,omitempty"`
}

const dashAISystemPrompt = `Tu es l'assistant hop. Tu connais la config de l'utilisateur. Tu peux repondre en texte ou proposer une commande hop a executer. Si tu proposes une commande, prefixe-la avec CMD: sur une ligne separee. Reponds de maniere concise et utile.`

// dashSafeContext builds a context string from config, stripping secrets.
func dashSafeContext(cfg *config.Config) string {
	var sb strings.Builder
	sb.WriteString("=== Config hop ===\n")
	if config.IsInstalled() {
		sb.WriteString("Mode: installe (~/.hop/)\n")
	} else {
		sb.WriteString("Mode: sandbox\n")
	}
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
	if len(cfg.Aliases) > 0 {
		sb.WriteString("\nAliases:\n")
		for alias, target := range cfg.Aliases {
			sb.WriteString(fmt.Sprintf("  %s -> %s\n", alias, target))
		}
	}
	if cfg.Cloudflare.Domain != "" {
		sb.WriteString(fmt.Sprintf("\nCloudflare domain: %s\n", cfg.Cloudflare.Domain))
	}
	return sb.String()
}

// dashLoadCFCredentials reads CF_ACCOUNT_ID and CF_API_KEY from cloudflare.env.
func dashLoadCFCredentials(cfg *config.Config) (string, string, error) {
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

// dashAskWorkersAI sends a prompt to Cloudflare Workers AI.
func dashAskWorkersAI(accountID, apiKey, prompt string) (string, error) {
	// Validate account ID format (hex, 32 chars)
	if len(accountID) != 32 {
		return "", fmt.Errorf("account ID invalide")
	}
	for _, c := range accountID {
		if !((c >= 'a' && c <= 'f') || (c >= '0' && c <= '9')) {
			return "", fmt.Errorf("account ID invalide")
		}
	}
	payload := map[string]interface{}{
		"messages": []map[string]string{
			{"role": "system", "content": dashAISystemPrompt},
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

// dashParseResponse splits the LLM response into text + optional CMD: command.
func dashParseResponse(response string) (text, cmd string) {
	lines := strings.Split(strings.TrimSpace(response), "\n")
	var textLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "CMD:") {
			cmd = strings.TrimSpace(strings.TrimPrefix(trimmed, "CMD:"))
		} else {
			textLines = append(textLines, line)
		}
	}
	text = strings.TrimSpace(strings.Join(textLines, "\n"))
	return
}

func handleAI(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "method not allowed", 405)
		return
	}
	limitBody(w, r)

	var req aiReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "bad request", 400)
		return
	}
	if strings.TrimSpace(req.Question) == "" {
		jsonError(w, "question vide", 400)
		return
	}

	configMu.Lock()
	cfg, err := config.Load()
	configMu.Unlock()
	if err != nil {
		jsonError(w, "internal", 500)
		return
	}

	if !cfg.AIEnabled {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]string{"error": "ai_disabled"})
		return
	}

	ctx := dashSafeContext(cfg)
	fullPrompt := fmt.Sprintf("%s\n\n%s\n\nQuestion: %s", dashAISystemPrompt, ctx, req.Question)

	accountID, apiKey, cfErr := dashLoadCFCredentials(cfg)
	if cfErr != nil || accountID == "" || apiKey == "" {
		jsonError(w, "Cloudflare Workers AI non configure (hop config)", 503)
		return
	}

	rawResponse, err := dashAskWorkersAI(accountID, apiKey, fullPrompt)
	if err != nil {
		jsonError(w, "erreur AI", 503)
		return
	}
	source := "workers_ai"

	text, cmd := dashParseResponse(rawResponse)

	// Whitelist check on proposed command
	if cmd != "" {
		parts := strings.Fields(cmd)
		if len(parts) > 0 && parts[0] == "hop" {
			parts = parts[1:]
		}
		safe := map[string]bool{
			"ssh": true, "ping": true, "list": true,
			"send": true, "receive": true, "pair": true,
			"tunnel": true, "add": true, "alias": true,
			"dashboard": true, "version": true, "export": true,
		}
		if len(parts) == 0 || !safe[parts[0]] {
			cmd = "" // Block unsafe command
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(aiResp{
		Answer:  text,
		Command: cmd,
		Source:  source,
	})
}

type pairReq struct {
	PairToken string `json:"pair_token"` // format: pairID.code.token
}

func handlePair(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "method not allowed", 405)
		return
	}
	limitBody(w, r)

	var req pairReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "bad request", 400)
		return
	}

	// Token is now a simple 8-char code (v3)
	code := strings.ToLower(strings.TrimSpace(req.PairToken))
	if len(code) != 8 {
		jsonError(w, "code invalide", 400)
		return
	}

	// Fetch server's pair data
	serverData, err := pairing.FetchPairData(code)
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}

	if err := config.ValidateName(serverData.Hostname); err != nil {
		jsonError(w, "hostname distant invalide", 400)
		return
	}

	// Ensure local SSH key
	hostname, _ := os.Hostname()
	_, pubKey, err := pairing.EnsureSSHKey()
	if err != nil {
		jsonError(w, "cannot generate SSH key", 500)
		return
	}

	// Build and send response (domain only, not API key)
	user := os.Getenv("USER")
	response := &pairing.PairData{
		Hostname:  hostname,
		PublicKey: pubKey,
		User:      user,
	}

	configMu.Lock()
	cfg, err := config.Load()
	cfDomain := ""
	if err == nil && cfg.Cloudflare.Domain != "" {
		cfDomain = cfg.Cloudflare.Domain
		response.CFDomain = cfDomain
	}
	configMu.Unlock()

	session := &pairing.PairSession{Code: code}
	if err := pairing.SendResponse(session, response); err != nil {
		jsonError(w, "erreur envoi reponse", 500)
		return
	}

	// Add server's key locally
	if err := pairing.AddAuthorizedKey(serverData.PublicKey); err != nil {
		jsonError(w, "erreur ajout cle SSH", 500)
		return
	}

	// Add machine to config
	tunnel := ""
	if cfDomain != "" {
		tunnel = serverData.Hostname + "." + cfDomain
	}

	configMu.Lock()
	defer configMu.Unlock()

	cfg, err = config.Load()
	if err != nil {
		jsonError(w, "internal", 500)
		return
	}

	cfg.Machines[serverData.Hostname] = config.Machine{
		IP:       serverData.IP,
		User:     serverData.User,
		Tunnel:   tunnel,
		Services: make(map[string]config.MachineService),
	}

	if err := cfg.Save(); err != nil {
		jsonError(w, "internal", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":       true,
		"hostname": serverData.Hostname,
	})
}

func handlePairStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "method not allowed", 405)
		return
	}

	activePairMu.Lock()
	defer activePairMu.Unlock()

	// Clean up any previous session
	if activePairSession != nil {
		pairing.Cleanup(activePairSession)
		activePairSession = nil

		activePairResult = nil
	}

	hostname, _ := os.Hostname()
	_, pubKey, err := pairing.EnsureSSHKey()
	if err != nil {
		jsonError(w, "cannot generate SSH key", 500)
		return
	}

	user := os.Getenv("USER")
	if user == "" {
		user = "hop"
	}
	code := pairing.GenerateCode()

	data := &pairing.PairData{
		Hostname:  hostname,
		PublicKey: pubKey,
		User:      user,
		Version:   DashboardVersion,
	}

	configMu.Lock()
	cfg, err := config.Load()
	if err == nil && cfg.Cloudflare.Domain != "" {
		data.CFDomain = cfg.Cloudflare.Domain
	}
	configMu.Unlock()

	session, err := pairing.PublishPairData(code, data)
	if err != nil {
		jsonError(w, "erreur connexion relay", 500)
		return
	}

	activePairSession = session

	activePairResult = nil

	// Start polling in background
	go func() {
		result, err := pairing.WaitForResponse(session, 2*time.Minute)
		activePairMu.Lock()
		defer activePairMu.Unlock()
		if err != nil {
			// Timeout or error — clean up
			if activePairSession == session {
				pairing.Cleanup(session)
				activePairSession = nil

			}
			return
		}
		activePairResult = result

		// Auto-finalize: add key + machine
		_ = pairing.AddAuthorizedKey(result.PublicKey)

		configMu.Lock()
		defer configMu.Unlock()
		cfg, err := config.Load()
		if err == nil {
			tunnel := ""
			if cfg.Cloudflare.Domain != "" {
				tunnel = result.Hostname + "." + cfg.Cloudflare.Domain
			}
			cfg.Machines[result.Hostname] = config.Machine{
				IP:       result.IP,
				User:     result.User,
				Tunnel:   tunnel,
				Services: make(map[string]config.MachineService),
			}
			cfg.Save()
		}
		pairing.Cleanup(session)
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":    true,
		"token": code,
		"code":  code,
	})
}

func handlePairStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		jsonError(w, "method not allowed", 405)
		return
	}

	activePairMu.Lock()
	defer activePairMu.Unlock()

	if activePairSession == nil && activePairResult == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "none",
		})
		return
	}

	if activePairResult != nil {
		hostname := activePairResult.Hostname
		activePairSession = nil

		activePairResult = nil
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "paired",
			"hostname": hostname,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "waiting",
	})
}
