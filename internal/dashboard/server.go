package dashboard

import (
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

	"github.com/meumeu-dev/hop/internal/config"
)

//go:embed static/*
var staticFiles embed.FS

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

// authMiddleware checks API key for /api/ routes (except /api/ping which is public)
func authMiddleware(apiKey string, readOnly bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Dashboard static files — no auth (localhost only)
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

		// Check API key from header or query param
		key := r.Header.Get("X-Hop-Key")
		if key == "" {
			key = r.URL.Query().Get("key")
		}

		if key != apiKey {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"unauthorized","message":"Clé API invalide. Utilise --key lors de hop remote add."}`, 401)
			return
		}

		// Read-only mode: block write operations
		if readOnly && r.Method != "GET" {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"forbidden","message":"API en mode lecture seule."}`, 403)
			return
		}

		next.ServeHTTP(w, r)
	})
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

	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("port %d déjà utilisé: %w", port, err)
	}

	url := fmt.Sprintf("http://localhost:%d", port)
	fmt.Printf("→ Dashboard sur %s\n", url)

	if open {
		openBrowser(url)
	}

	// Dashboard runs without auth (localhost)
	return http.Serve(listener, mux)
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

	handler := authMiddleware(cfg.API.Key, cfg.API.ReadOnly, mux)

	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("port %d déjà utilisé: %w", port, err)
	}

	fmt.Printf("→ API hop sur le port %d\n", port)
	fmt.Printf("→ Clé API: %s\n", cfg.API.Key)
	if cfg.API.ReadOnly {
		fmt.Println("→ Mode: lecture seule")
	} else {
		fmt.Println("→ Mode: lecture + écriture")
	}
	fmt.Println()
	fmt.Println("Pour connecter un client:")
	fmt.Printf("  hop remote add <nom> http://<ip>:%d --key %s\n", port, cfg.API.Key)

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
	cfg, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// Don't expose API key or remote keys in response
	safeCfg := *cfg
	safeCfg.API = config.APIConfig{}
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
		http.Error(w, "Method not allowed", 405)
		return
	}

	var req machineReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	cfg, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), 500)
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
		http.Error(w, err.Error(), 500)
		return
	}

	w.WriteHeader(200)
	fmt.Fprintf(w, `{"ok":true}`)
}

func handleMachineDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/machines/")

	cfg, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	delete(cfg.Machines, name)

	if err := cfg.Save(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.WriteHeader(200)
	fmt.Fprintf(w, `{"ok":true}`)
}

func handleServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var req serviceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	cfg, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	cfg.Services[req.Name] = config.Service{
		Desc: req.Desc,
		Cmd:  req.Cmd,
	}

	if err := cfg.Save(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.WriteHeader(200)
	fmt.Fprintf(w, `{"ok":true}`)
}

func handleServiceDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/services/")

	cfg, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	if svc, ok := cfg.Services[name]; ok && svc.Builtin {
		http.Error(w, "Cannot delete builtin service", 400)
		return
	}

	delete(cfg.Services, name)

	if err := cfg.Save(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.WriteHeader(200)
	fmt.Fprintf(w, `{"ok":true}`)
}

func handleRemotes(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var req remoteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	cfg, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	if cfg.Remotes == nil {
		cfg.Remotes = make(map[string]config.Remote)
	}

	cfg.Remotes[req.Name] = config.Remote{URL: req.URL, Key: req.Key}

	if err := cfg.Save(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.WriteHeader(200)
	fmt.Fprintf(w, `{"ok":true}`)
}

func handleRemoteRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/remotes/")

	if r.Method == "DELETE" && !strings.Contains(path, "/") {
		handleRemoteDelete(w, r, path)
		return
	}

	if strings.HasPrefix(path, "ping/") {
		handleRemotePing(w, r, strings.TrimPrefix(path, "ping/"))
		return
	}

	if strings.HasPrefix(path, "config/") {
		handleRemoteConfig(w, r, strings.TrimPrefix(path, "config/"))
		return
	}

	http.Error(w, "Not found", 404)
}

func handleRemoteDelete(w http.ResponseWriter, r *http.Request, name string) {
	cfg, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	delete(cfg.Remotes, name)

	if err := cfg.Save(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.WriteHeader(200)
	fmt.Fprintf(w, `{"ok":true}`)
}

func handleRemotePing(w http.ResponseWriter, r *http.Request, name string) {
	cfg, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	remote, ok := cfg.Remotes[name]
	if !ok {
		http.Error(w, "Remote not found", 404)
		return
	}

	// Ping is public, no key needed
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
	cfg, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	remote, ok := cfg.Remotes[name]
	if !ok {
		http.Error(w, "Remote not found", 404)
		return
	}

	resp, err := remoteGet(remote, "/api/config")
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		http.Error(w, `{"error":"unauthorized","message":"Clé API invalide pour ce remote."}`, 401)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}
