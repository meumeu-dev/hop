package dashboard

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/meumeu-dev/hop/internal/config"
)

//go:embed static/*
var staticFiles embed.FS

// Mutex to prevent race conditions on config file
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

type remoteReq struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Key  string `json:"key"`
}

// authMiddleware checks API key for /api/ routes
func authMiddleware(apiKey string, readOnly bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Static files — no auth
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		// Ping is always public
		if r.URL.Path == "/api/ping" {
			next.ServeHTTP(w, r)
			return
		}

		// If no API key configured, allow all (local dashboard mode)
		if apiKey == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Check API key from header only (not query params to prevent leaking)
		key := r.Header.Get("X-Hop-Key")

		// Constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) != 1 {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"unauthorized"}`, 401)
			return
		}

		// Read-only mode: block write operations
		if readOnly && r.Method != "GET" {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"forbidden"}`, 403)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// CSRF + CORS middleware for dashboard
func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")

		// For API mutating requests, check Origin header (CSRF protection)
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Method != "GET" {
			origin := r.Header.Get("Origin")
			if origin != "" && !strings.HasPrefix(origin, "http://localhost") && !strings.HasPrefix(origin, "http://127.0.0.1") {
				http.Error(w, `{"error":"csrf"}`, 403)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// limitBody wraps request body with a size limit
func limitBody(r *http.Request) {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20) // 1MB
}

func registerAPIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/ping", handlePing)
	mux.HandleFunc("/api/config", handleConfig)
	mux.HandleFunc("/api/machines", handleMachines)
	mux.HandleFunc("/api/machines/", handleMachineDelete)
	mux.HandleFunc("/api/services", handleServices)
	mux.HandleFunc("/api/services/", handleServiceDelete)
	mux.HandleFunc("/api/remotes", handleRemotes)
	mux.HandleFunc("/api/remotes/", handleRemoteRoute)
}

func Start(port int, open bool) error {
	mux := http.NewServeMux()
	registerAPIRoutes(mux)

	staticFS, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	// Dashboard binds to localhost ONLY
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("port %d déjà utilisé: %w", port, err)
	}

	url := fmt.Sprintf("http://localhost:%d", port)
	fmt.Printf("→ Dashboard sur %s (localhost uniquement)\n", url)

	if open {
		openBrowser(url)
	}

	handler := securityMiddleware(mux)
	return http.Serve(listener, handler)
}

func StartAPI(port int) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Generate API key if not set
	if cfg.API.Key == "" {
		cfg.API.Key = config.GenerateAPIKey()
		cfg.Save()
	}

	mux := http.NewServeMux()
	registerAPIRoutes(mux)

	handler := securityMiddleware(authMiddleware(cfg.API.Key, cfg.API.ReadOnly, mux))

	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("port %d déjà utilisé: %w", port, err)
	}

	fmt.Printf("→ API hop sur le port %d\n", port)
	// Show only first/last 4 chars of key
	key := cfg.API.Key
	maskedKey := key[:4] + "..." + key[len(key)-4:]
	fmt.Printf("→ Clé API: %s (hop api --show-key pour la clé complète)\n", maskedKey)
	if cfg.API.ReadOnly {
		fmt.Println("→ Mode: lecture seule")
	} else {
		fmt.Println("→ Mode: lecture + écriture")
	}
	fmt.Println()
	fmt.Printf("Pour connecter un client: hop remote add <nom> http://<ip>:%d --key <clé>\n", port)

	return http.Serve(listener, handler)
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

// --- Helpers for remote requests with auth ---

func remoteGet(remote config.Remote, path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", remote.URL+path, nil)
	if err != nil {
		return nil, err
	}
	if remote.Key != "" {
		req.Header.Set("X-Hop-Key", remote.Key)
	}
	return http.DefaultClient.Do(req)
}

// --- Handlers ---

func handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","version":"0.1.0"}`)
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	configMu.Lock()
	cfg, err := config.Load()
	configMu.Unlock()
	if err != nil {
		http.Error(w, `{"error":"internal"}`, 500)
		return
	}

	// Strip secrets from response
	safeCfg := *cfg
	safeCfg.API = config.APIConfig{}
	safeCfg.Cloudflare = config.CloudflareConfig{Domain: cfg.Cloudflare.Domain}
	safeRemotes := make(map[string]config.Remote)
	for k, v := range cfg.Remotes {
		safeRemotes[k] = config.Remote{URL: v.URL}
	}
	safeCfg.Remotes = safeRemotes

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(safeCfg)
}

func handleMachines(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}
	limitBody(r)

	var req machineReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad request"}`, 400)
		return
	}

	// Validate inputs
	if err := config.ValidateName(req.Name); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), 400)
		return
	}
	if err := config.ValidateIP(req.IP); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), 400)
		return
	}
	if err := config.ValidateUser(req.User); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), 400)
		return
	}
	if err := config.ValidateTunnel(req.Tunnel); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), 400)
		return
	}

	configMu.Lock()
	defer configMu.Unlock()

	cfg, err := config.Load()
	if err != nil {
		http.Error(w, `{"error":"internal"}`, 500)
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
		http.Error(w, `{"error":"internal"}`, 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ok":true}`)
}

func handleMachineDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/machines/")
	if err := config.ValidateName(name); err != nil {
		http.Error(w, `{"error":"invalid name"}`, 400)
		return
	}

	configMu.Lock()
	defer configMu.Unlock()

	cfg, err := config.Load()
	if err != nil {
		http.Error(w, `{"error":"internal"}`, 500)
		return
	}

	delete(cfg.Machines, name)

	if err := cfg.Save(); err != nil {
		http.Error(w, `{"error":"internal"}`, 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ok":true}`)
}

func handleServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}
	limitBody(r)

	var req serviceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad request"}`, 400)
		return
	}

	if err := config.ValidateName(req.Name); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), 400)
		return
	}

	configMu.Lock()
	defer configMu.Unlock()

	cfg, err := config.Load()
	if err != nil {
		http.Error(w, `{"error":"internal"}`, 500)
		return
	}

	// Don't allow overwriting builtin services
	if svc, ok := cfg.Services[req.Name]; ok && svc.Builtin {
		http.Error(w, `{"error":"cannot modify builtin service"}`, 400)
		return
	}

	cfg.Services[req.Name] = config.Service{
		Desc: req.Desc,
		Cmd:  req.Cmd,
	}

	if err := cfg.Save(); err != nil {
		http.Error(w, `{"error":"internal"}`, 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ok":true}`)
}

func handleServiceDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/services/")
	if err := config.ValidateName(name); err != nil {
		http.Error(w, `{"error":"invalid name"}`, 400)
		return
	}

	configMu.Lock()
	defer configMu.Unlock()

	cfg, err := config.Load()
	if err != nil {
		http.Error(w, `{"error":"internal"}`, 500)
		return
	}

	if svc, ok := cfg.Services[name]; ok && svc.Builtin {
		http.Error(w, `{"error":"cannot delete builtin service"}`, 400)
		return
	}

	delete(cfg.Services, name)

	if err := cfg.Save(); err != nil {
		http.Error(w, `{"error":"internal"}`, 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ok":true}`)
}

func handleRemotes(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, 405)
		return
	}
	limitBody(r)

	var req remoteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad request"}`, 400)
		return
	}

	if err := config.ValidateName(req.Name); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), 400)
		return
	}
	if err := config.ValidateURL(req.URL); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), 400)
		return
	}

	configMu.Lock()
	defer configMu.Unlock()

	cfg, err := config.Load()
	if err != nil {
		http.Error(w, `{"error":"internal"}`, 500)
		return
	}

	if cfg.Remotes == nil {
		cfg.Remotes = make(map[string]config.Remote)
	}

	cfg.Remotes[req.Name] = config.Remote{URL: req.URL, Key: req.Key}

	if err := cfg.Save(); err != nil {
		http.Error(w, `{"error":"internal"}`, 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ok":true}`)
}

func handleRemoteRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/remotes/")

	if r.Method == "DELETE" && !strings.Contains(path, "/") {
		if err := config.ValidateName(path); err != nil {
			http.Error(w, `{"error":"invalid name"}`, 400)
			return
		}
		handleRemoteDelete(w, r, path)
		return
	}

	if strings.HasPrefix(path, "ping/") {
		name := strings.TrimPrefix(path, "ping/")
		if err := config.ValidateName(name); err != nil {
			http.Error(w, `{"error":"invalid name"}`, 400)
			return
		}
		handleRemotePing(w, r, name)
		return
	}

	if strings.HasPrefix(path, "config/") {
		name := strings.TrimPrefix(path, "config/")
		if err := config.ValidateName(name); err != nil {
			http.Error(w, `{"error":"invalid name"}`, 400)
			return
		}
		handleRemoteConfig(w, r, name)
		return
	}

	http.Error(w, `{"error":"not found"}`, 404)
}

func handleRemoteDelete(w http.ResponseWriter, r *http.Request, name string) {
	configMu.Lock()
	defer configMu.Unlock()

	cfg, err := config.Load()
	if err != nil {
		http.Error(w, `{"error":"internal"}`, 500)
		return
	}

	delete(cfg.Remotes, name)

	if err := cfg.Save(); err != nil {
		http.Error(w, `{"error":"internal"}`, 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ok":true}`)
}

func handleRemotePing(w http.ResponseWriter, r *http.Request, name string) {
	configMu.Lock()
	cfg, err := config.Load()
	configMu.Unlock()
	if err != nil {
		http.Error(w, `{"error":"internal"}`, 500)
		return
	}

	remote, ok := cfg.Remotes[name]
	if !ok {
		http.Error(w, `{"error":"not found"}`, 404)
		return
	}

	resp, err := http.Get(remote.URL + "/api/ping")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"offline"}`)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	if resp.StatusCode == 200 {
		fmt.Fprintf(w, `{"status":"online"}`)
	} else {
		fmt.Fprintf(w, `{"status":"offline"}`)
	}
}

func handleRemoteConfig(w http.ResponseWriter, r *http.Request, name string) {
	configMu.Lock()
	cfg, err := config.Load()
	configMu.Unlock()
	if err != nil {
		http.Error(w, `{"error":"internal"}`, 500)
		return
	}

	remote, ok := cfg.Remotes[name]
	if !ok {
		http.Error(w, `{"error":"not found"}`, 404)
		return
	}

	resp, err := remoteGet(remote, "/api/config")
	if err != nil {
		http.Error(w, `{"error":"remote unreachable"}`, 502)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, io.LimitReader(resp.Body, 1<<20)) // 1MB max
}
