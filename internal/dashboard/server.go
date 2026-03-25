package dashboard

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
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

func Start(port int, open bool) error {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/config", handleConfig)
	mux.HandleFunc("/api/machines", handleMachines)
	mux.HandleFunc("/api/machines/", handleMachineDelete)
	mux.HandleFunc("/api/services", handleServices)
	mux.HandleFunc("/api/services/", handleServiceDelete)

	// Static files
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

	return http.Serve(listener, mux)
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

func handleConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
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

func init() {
	// Suppress unused import
	_ = os.Stderr
}
