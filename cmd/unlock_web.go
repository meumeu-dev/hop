package cmd

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

// Page servie par le serveur d'unlock. Le chiffrement de la passphrase se fait
// dans le navigateur (WebCrypto RSA-OAEP) avec la cle publique ephemere ci-
// dessous : Cloudflare, qui termine le TLS a son edge, ne voit qu'un blob
// chiffre, jamais la passphrase en clair.
//
//go:embed unlock_web.html
var unlockWebHTML string

var (
	webAddr        string
	webCryptroot   string
	webMaxAttempts int
)

// unlockWebCmd lance, DEPUIS l'initramfs, un petit serveur HTTP local qui sert
// une page ou saisir la passphrase LUKS depuis n'importe quel navigateur. Il
// n'ecoute que sur la boucle locale : il n'est joignable qu'a travers le tunnel
// Cloudflare (ingress http://127.0.0.1:<port>), lui-meme protege par Cloudflare
// Access. La passphrase arrive chiffree (RSA-OAEP) et n'est dechiffree qu'ici,
// en RAM, avec une cle privee ephemere generee a chaque boot et jamais ecrite
// sur disque.
var unlockWebCmd = &cobra.Command{
	Use:    "web",
	Short:  "Sert une page web locale de deverrouillage (usage initramfs)",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		srv, err := newUnlockWebServer(webCryptroot, webMaxAttempts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hop unlock web: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("hop unlock web: ecoute sur %s (cryptroot-unlock=%s)\n", webAddr, webCryptroot)
		httpSrv := &http.Server{
			Addr:              webAddr,
			Handler:           srv.routes(),
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      2 * time.Minute, // cryptroot-unlock peut mettre du temps
		}
		if err := httpSrv.ListenAndServe(); err != nil {
			fmt.Fprintf(os.Stderr, "hop unlock web: %v\n", err)
			os.Exit(1)
		}
	},
}

type unlockWebServer struct {
	priv          *rsa.PrivateKey
	pubDER        []byte
	cryptrootPath string
	maxAttempts   int

	mu       sync.Mutex // serialise les tentatives (une seule a la fois)
	attempts int
	done     bool // passe a true une fois le disque deverrouille
}

func newUnlockWebServer(cryptrootPath string, maxAttempts int) (*unlockWebServer, error) {
	// Cle RSA ephemere : generee ici, en RAM, a chaque demarrage du serveur.
	// La cle privee ne quitte jamais ce process et n'est jamais persistee (la
	// partition /boot n'est pas chiffree, on n'y ecrit donc aucun secret).
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generation cle: %w", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("serialisation cle publique: %w", err)
	}
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	return &unlockWebServer{
		priv:          priv,
		pubDER:        pubDER,
		cryptrootPath: cryptrootPath,
		maxAttempts:   maxAttempts,
	}, nil
}

func (s *unlockWebServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/pubkey", s.handlePubkey)
	mux.HandleFunc("/unlock", s.handleUnlock)
	return securityHeaders(mux)
}

// securityHeaders durcit un minimum la page. La CSP interdit tout script/asset
// externe : le seul JS execute est celui, inline, de notre page (WebCrypto).
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; form-action 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (s *unlockWebServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, unlockWebHTML)
}

func (s *unlockWebServer) handlePubkey(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"alg":    "RSA-OAEP-256",
		"pubkey": base64.StdEncoding.EncodeToString(s.pubDER),
	})
}

type unlockRequest struct {
	Blob string `json:"blob"` // ciphertext RSA-OAEP(SHA-256), base64 standard
}

type unlockResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Msg   string `json:"msg,omitempty"`
}

func (s *unlockWebServer) handleUnlock(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(unlockResponse{Error: "methode non autorisee"})
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.done {
		json.NewEncoder(w).Encode(unlockResponse{OK: true, Msg: "deja deverrouille"})
		return
	}
	if s.attempts >= s.maxAttempts {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(unlockResponse{Error: "trop de tentatives, reboote la machine"})
		return
	}

	var req unlockRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(unlockResponse{Error: "requete invalide"})
		return
	}
	ciphertext, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.Blob))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(unlockResponse{Error: "blob invalide"})
		return
	}

	// Dechiffrement en RAM. La passphrase n'existe en clair que dans ce buffer,
	// remis a zero des qu'on a fini.
	pass, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, s.priv, ciphertext, nil)
	if err != nil {
		s.attempts++
		time.Sleep(1 * time.Second) // anti-bruteforce (CF Access protege deja l'acces)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(unlockResponse{Error: "dechiffrement impossible (cle publique perimee ? recharge la page)"})
		return
	}
	defer zero(pass)

	if len(pass) == 0 {
		s.attempts++
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(unlockResponse{Error: "passphrase vide"})
		return
	}

	s.attempts++
	if err := s.feedCryptroot(pass); err != nil {
		time.Sleep(1 * time.Second)
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(unlockResponse{Error: "passphrase refusee, reessaie"})
		return
	}

	s.done = true
	json.NewEncoder(w).Encode(unlockResponse{OK: true, Msg: "disque deverrouille, le boot continue"})
}

// feedCryptroot injecte la passphrase exactement comme le fait la voie SSH :
// il lance /usr/bin/cryptroot-unlock qui, en mode non-interactif (stdin non
// TTY), recopie tel quel stdin dans /lib/cryptsetup/passfifo. On envoie donc la
// passphrase SANS retour a la ligne final (comme `printf '%s'` cote TTY).
// Sortie 0 => un device deverrouille ; sortie != 0 => mauvaise passphrase.
func (s *unlockWebServer) feedCryptroot(pass []byte) error {
	c := exec.Command(s.cryptrootPath)
	c.Stdin = strings.NewReader(string(pass))
	out, err := c.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hop unlock web: cryptroot-unlock echec: %v (%s)\n", err, strings.TrimSpace(string(out)))
		return err
	}
	fmt.Printf("hop unlock web: %s\n", strings.TrimSpace(string(out)))
	return nil
}

// zero efface un buffer sensible (best-effort, subtil pour eviter que le
// compilateur elimine l'ecriture).
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
	_ = subtle.ConstantTimeByteEq(0, b[0])
}

func init() {
	unlockWebCmd.Flags().StringVar(&webAddr, "addr", "127.0.0.1:8088", "adresse d'ecoute (boucle locale)")
	unlockWebCmd.Flags().StringVar(&webCryptroot, "cryptroot", "/usr/bin/cryptroot-unlock", "chemin de cryptroot-unlock")
	unlockWebCmd.Flags().IntVar(&webMaxAttempts, "max-attempts", 10, "nombre max de tentatives avant blocage")
	unlockCmd.AddCommand(unlockWebCmd)
}
