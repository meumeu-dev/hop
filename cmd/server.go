package cmd

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

var serverPort int

type pairEntry struct {
	Data      string    `json:"data"`
	TokenHash string    `json:"-"`
	Response  string    `json:"-"`
	CreatedAt time.Time `json:"-"`
}

type tunnelEntry struct {
	URL       string    `json:"url"`
	TokenHash string    `json:"-"`
	CreatedAt time.Time `json:"-"`
}

type relayStore struct {
	mu      sync.RWMutex
	pairs   map[string]*pairEntry
	tunnels map[string]*tunnelEntry
}

func newRelayStore() *relayStore {
	return &relayStore{
		pairs:   make(map[string]*pairEntry),
		tunnels: make(map[string]*tunnelEntry),
	}
}

const maxTunnels = 500

func (s *relayStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, p := range s.pairs {
		if now.Sub(p.CreatedAt) > 2*time.Minute {
			delete(s.pairs, id)
		}
	}
	for id, t := range s.tunnels {
		if now.Sub(t.CreatedAt) > 1*time.Hour {
			delete(s.tunnels, id)
		}
	}
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

const maxBodySize = 1 << 20 // 1MB
const maxPairs = 1000

var validMachineID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

func (s *relayStore) handlePairCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	var body struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Data == "" {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	pairID := generateID()
	token := generateToken()

	s.mu.Lock()
	if len(s.pairs) >= maxPairs {
		s.mu.Unlock()
		http.Error(w, "Too many active pairs", http.StatusTooManyRequests)
		return
	}
	s.pairs[pairID] = &pairEntry{
		Data:      body.Data,
		TokenHash: hashToken(token),
		CreatedAt: time.Now(),
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"pair_id": pairID,
		"token":   token,
	})
}

func (s *relayStore) handlePairGet(w http.ResponseWriter, r *http.Request, pairID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	entry, ok := s.pairs[pairID]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"data": entry.Data,
	})
}

func (s *relayStore) handlePairDelete(w http.ResponseWriter, r *http.Request, pairID string) {
	token := r.Header.Get("X-Pair-Token")
	if token == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.pairs[pairID]
	if !ok {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if hashToken(token) != entry.TokenHash {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	delete(s.pairs, pairID)
	w.WriteHeader(http.StatusOK)
}

func (s *relayStore) handleResponsePost(w http.ResponseWriter, r *http.Request, pairID string) {
	token := r.Header.Get("X-Pair-Token")
	if token == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.pairs[pairID]
	if !ok {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if hashToken(token) != entry.TokenHash {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if entry.Response != "" {
		http.Error(w, "Conflict", http.StatusConflict)
		return
	}

	var body struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Data == "" {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	entry.Response = body.Data
	w.WriteHeader(http.StatusOK)
}

func (s *relayStore) handleResponseGet(w http.ResponseWriter, r *http.Request, pairID string) {
	token := r.Header.Get("X-Pair-Token")
	if token == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	s.mu.RLock()
	entry, ok := s.pairs[pairID]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if hashToken(token) != entry.TokenHash {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if entry.Response == "" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"data": entry.Response,
	})
}

func (s *relayStore) handleTunnelRegister(w http.ResponseWriter, r *http.Request, machineID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	var body struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" || body.Token == "" {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	tokenHash := hashToken(body.Token)

	s.mu.Lock()
	// Check if already registered with different token
	if existing, ok := s.tunnels[machineID]; ok {
		if existing.TokenHash != "" && existing.TokenHash != tokenHash {
			s.mu.Unlock()
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}
	if len(s.tunnels) >= maxTunnels {
		s.mu.Unlock()
		http.Error(w, "Too many tunnels", http.StatusTooManyRequests)
		return
	}
	s.tunnels[machineID] = &tunnelEntry{
		URL:       body.URL,
		TokenHash: tokenHash,
		CreatedAt: time.Now(),
	}
	s.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (s *relayStore) handleTunnelResolve(w http.ResponseWriter, r *http.Request, machineID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	entry, ok := s.tunnels[machineID]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"url": entry.URL,
	})
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Lance un serveur relay de pairing autonome",
	Long:  `Lance un serveur HTTP qui sert de relay pour le pairing, comme alternative au Cloudflare Worker.`,
	Run: func(cmd *cobra.Command, args []string) {
		store := newRelayStore()

		// Auto-cleanup goroutine
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				store.cleanup()
			}
		}()

		mux := http.NewServeMux()

		// POST /pair
		mux.HandleFunc("/pair", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				store.handlePairCreate(w, r)
				return
			}
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		})

		// GET/DELETE /pair/:id and POST/GET /pair/:id/response
		mux.HandleFunc("/pair/", func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimPrefix(r.URL.Path, "/pair/")
			parts := strings.SplitN(path, "/", 2)
			pairID := parts[0]

			if pairID == "" {
				http.Error(w, "Not found", http.StatusNotFound)
				return
			}

			if len(parts) == 2 && parts[1] == "response" {
				switch r.Method {
				case http.MethodPost:
					store.handleResponsePost(w, r, pairID)
				case http.MethodGet:
					store.handleResponseGet(w, r, pairID)
				default:
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
				return
			}

			switch r.Method {
			case http.MethodGet:
				store.handlePairGet(w, r, pairID)
			case http.MethodDelete:
				store.handlePairDelete(w, r, pairID)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})

		// POST/GET /tunnel/:machine_id
		mux.HandleFunc("/tunnel/", func(w http.ResponseWriter, r *http.Request) {
			machineID := strings.TrimPrefix(r.URL.Path, "/tunnel/")
			if machineID == "" || !validMachineID.MatchString(machineID) {
				http.Error(w, "Not found", http.StatusNotFound)
				return
			}

			switch r.Method {
			case http.MethodPost:
				store.handleTunnelRegister(w, r, machineID)
			case http.MethodGet:
				store.handleTunnelResolve(w, r, machineID)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})

		// GET /health
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"status": "ok",
			})
		})

		addr := fmt.Sprintf(":%d", serverPort)
		fmt.Printf("→ Serveur relay hop démarré sur %s\n", addr)
		fmt.Println("→ Configure worker_url dans ~/.hop/config.yml pour l'utiliser")
		fmt.Printf("→ Exemple: worker_url: \"http://<ip>%s\"\n", addr)

		server := &http.Server{
			Addr:         addr,
			Handler:      mux,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		}

		if err := server.ListenAndServe(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur serveur: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	serverCmd.Flags().IntVar(&serverPort, "port", 8899, "Port d'écoute du serveur")
	rootCmd.AddCommand(serverCmd)
}
