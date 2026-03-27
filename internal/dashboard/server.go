package dashboard

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"path/filepath"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/meumeu-dev/hop/internal/pairing"
)

//go:embed static/*
var staticFiles embed.FS

// DashboardVersion is set from cmd package
var DashboardVersion = "dev"

var configMu sync.Mutex

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

func limitBody(r *http.Request) {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
}

func registerAPIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/ping", handlePing)
	mux.HandleFunc("/api/config", handleConfig)
	mux.HandleFunc("/api/machines", handleMachines)
	mux.HandleFunc("/api/machines/", handleMachineDelete)
	mux.HandleFunc("/api/services", handleServices)
	mux.HandleFunc("/api/services/", handleServiceDelete)
	mux.HandleFunc("/api/cloudflare", handleCloudflare)
	mux.HandleFunc("/api/pair", handlePair)
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
		WriteTimeout: 30 * time.Second,
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
	configMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(safeCfg)
}

func handleMachines(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "method not allowed", 405)
		return
	}
	limitBody(r)

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

func handleServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "method not allowed", 405)
		return
	}
	limitBody(r)

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
	limitBody(r)

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

	// Write env file
	envPath := filepath.Join(config.HopDir(), "cloudflare.env")
	envContent := fmt.Sprintf("CF_USER=%s\nCF_DOMAIN=%s\nCF_API_KEY=%s\n", req.Email, req.Domain, req.APIKey)
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

type pairReq struct {
	PairToken string `json:"pair_token"` // format: pairID.code.token
}

func handlePair(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "method not allowed", 405)
		return
	}
	limitBody(r)

	var req pairReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "bad request", 400)
		return
	}

	// Parse pair token
	parts := strings.SplitN(req.PairToken, ".", 3)
	if len(parts) != 3 {
		jsonError(w, "token invalide", 400)
		return
	}
	pairID, code, token := parts[0], parts[1], parts[2]

	// Fetch server's pair data
	serverData, err := pairing.FetchPairData(pairID, code)
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

	session := &pairing.PairSession{PairID: pairID, Token: token, Code: code}
	if err := pairing.SendResponse(session, response); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// Add server's key locally
	if err := pairing.AddAuthorizedKey(serverData.PublicKey); err != nil {
		jsonError(w, err.Error(), 500)
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
