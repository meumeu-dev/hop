package dashboard

import (
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
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/meumeu-dev/hop/internal/config"
)

//go:embed static/*
var staticFiles embed.FS

var configMu sync.Mutex

// HTTP client that does NOT follow redirects (SSRF prevention)
var safeClient = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

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

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func jsonOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// authMiddleware for API mode (hop api)
func authMiddleware(apiKey string, readOnly bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/api/ping" {
			next.ServeHTTP(w, r)
			return
		}
		if apiKey == "" {
			next.ServeHTTP(w, r)
			return
		}

		key := r.Header.Get("X-Hop-Key")
		if subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) != 1 {
			jsonError(w, "unauthorized", 401)
			return
		}
		if readOnly && r.Method != "GET" {
			jsonError(w, "forbidden", 403)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// dashboardAuthMiddleware generates a per-session CSRF token for the dashboard
func dashboardAuthMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")

		if strings.HasPrefix(r.URL.Path, "/api/") && r.Method != "GET" {
			// Require CSRF token for mutations
			csrfToken := r.Header.Get("X-Hop-CSRF")
			if subtle.ConstantTimeCompare([]byte(csrfToken), []byte(token)) != 1 {
				jsonError(w, "csrf", 403)
				return
			}
		}

		// Inject CSRF token endpoint
		if r.URL.Path == "/api/csrf" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"token": token})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
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
	mux.HandleFunc("/api/remotes", handleRemotes)
	mux.HandleFunc("/api/remotes/", handleRemoteRoute)
}

func Start(port int, open bool) error {
	mux := http.NewServeMux()
	registerAPIRoutes(mux)

	staticFS, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	// Generate per-session CSRF token for dashboard
	tokenBytes := make([]byte, 32)
	rand.Read(tokenBytes)
	csrfToken := hex.EncodeToString(tokenBytes)

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("port %d deja utilise: %w", port, err)
	}

	url := fmt.Sprintf("http://localhost:%d", port)
	fmt.Printf("-> Dashboard sur %s (localhost uniquement)\n", url)

	if open {
		openBrowser(url)
	}

	handler := dashboardAuthMiddleware(csrfToken, mux)
	return http.Serve(listener, handler)
}

func StartAPI(port int) error {
	secrets, _ := config.LoadSecrets()

	if secrets.APIKey == "" {
		secrets.APIKey = config.GenerateAPIKey()
		secrets.Save()
	}

	mux := http.NewServeMux()
	registerAPIRoutes(mux)

	handler := securityMiddleware(authMiddleware(secrets.APIKey, false, mux))

	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("port %d deja utilise: %w", port, err)
	}

	fmt.Printf("-> API hop sur le port %d\n", port)
	key := secrets.APIKey
	maskedKey := key[:4] + "..." + key[len(key)-4:]
	fmt.Printf("-> Cle API: %s (hop api --show-key pour la cle complete)\n", maskedKey)
	fmt.Println()
	fmt.Printf("Pour connecter un client: hop remote add <nom> http://<ip>:%d --key <cle>\n", port)

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

func remoteGet(remote config.Remote, path string) (*http.Response, error) {
	secrets, _ := config.LoadSecrets()
	req, err := http.NewRequest("GET", remote.URL+path, nil)
	if err != nil {
		return nil, err
	}
	key := secrets.RemoteKeys[remote.URL]
	if key != "" {
		req.Header.Set("X-Hop-Key", key)
	}
	return safeClient.Do(req)
}

// --- Handlers ---

func handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","version":"0.2.0"}`)
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
	safeCfg.API = config.APIConfig{}
	safeCfg.Cloudflare = config.CloudflareConfig{Domain: cfg.Cloudflare.Domain}
	safeRemotes := make(map[string]config.Remote)
	for k, v := range cfg.Remotes {
		safeRemotes[k] = config.Remote{URL: v.URL}
	}
	safeCfg.Remotes = safeRemotes
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

func handleRemotes(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, "method not allowed", 405)
		return
	}
	limitBody(r)

	var req remoteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "bad request", 400)
		return
	}

	if err := config.ValidateName(req.Name); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	if err := config.ValidateURL(req.URL); err != nil {
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

	if cfg.Remotes == nil {
		cfg.Remotes = make(map[string]config.Remote)
	}

	cfg.Remotes[req.Name] = config.Remote{URL: req.URL}

	if err := cfg.Save(); err != nil {
		jsonError(w, "internal", 500)
		return
	}

	// Store key in secrets file
	if req.Key != "" {
		secrets, _ := config.LoadSecrets()
		secrets.RemoteKeys[req.URL] = req.Key
		secrets.Save()
	}

	jsonOK(w)
}

func handleRemoteRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/remotes/")

	if r.Method == "DELETE" && !strings.Contains(path, "/") {
		if err := config.ValidateName(path); err != nil {
			jsonError(w, "invalid name", 400)
			return
		}
		handleRemoteDelete(w, r, path)
		return
	}

	if strings.HasPrefix(path, "ping/") {
		name := strings.TrimPrefix(path, "ping/")
		if err := config.ValidateName(name); err != nil {
			jsonError(w, "invalid name", 400)
			return
		}
		handleRemotePing(w, r, name)
		return
	}

	if strings.HasPrefix(path, "config/") {
		name := strings.TrimPrefix(path, "config/")
		if err := config.ValidateName(name); err != nil {
			jsonError(w, "invalid name", 400)
			return
		}
		handleRemoteConfig(w, r, name)
		return
	}

	jsonError(w, "not found", 404)
}

func handleRemoteDelete(w http.ResponseWriter, r *http.Request, name string) {
	configMu.Lock()
	defer configMu.Unlock()

	cfg, err := config.Load()
	if err != nil {
		jsonError(w, "internal", 500)
		return
	}

	delete(cfg.Remotes, name)

	if err := cfg.Save(); err != nil {
		jsonError(w, "internal", 500)
		return
	}

	jsonOK(w)
}

func handleRemotePing(w http.ResponseWriter, r *http.Request, name string) {
	configMu.Lock()
	cfg, err := config.Load()
	configMu.Unlock()
	if err != nil {
		jsonError(w, "internal", 500)
		return
	}

	remote, ok := cfg.Remotes[name]
	if !ok {
		jsonError(w, "not found", 404)
		return
	}

	resp, err := safeClient.Get(remote.URL + "/api/ping")
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
		jsonError(w, "internal", 500)
		return
	}

	remote, ok := cfg.Remotes[name]
	if !ok {
		jsonError(w, "not found", 404)
		return
	}

	resp, err := remoteGet(remote, "/api/config")
	if err != nil {
		jsonError(w, "remote unreachable", 502)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		jsonError(w, "unauthorized", 401)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, io.LimitReader(resp.Body, 1<<20))
}
