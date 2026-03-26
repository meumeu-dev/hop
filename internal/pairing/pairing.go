package pairing

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/meumeu-dev/hop/internal/config"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/ssh"
)

const DefaultWorkerURL = "https://hop-pair.meumeudev.workers.dev"

// WorkerURL returns the configured worker URL or the default
func GetWorkerURL() string {
	// Check if custom worker is configured
	home, _ := os.UserHomeDir()
	data, err := os.ReadFile(home + "/.hop/config.yml")
	if err == nil {
		// Simple scan for worker_url in yaml
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "worker_url:") {
				url := strings.TrimSpace(strings.TrimPrefix(line, "worker_url:"))
				url = strings.Trim(url, "\"'")
				if url != "" {
					return url
				}
			}
		}
	}
	return DefaultWorkerURL
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

// PairData is what gets encrypted and sent through the worker
type PairData struct {
	Hostname  string   `json:"hostname"`
	IP        string   `json:"ip,omitempty"`
	IPs       []string `json:"ips,omitempty"`
	User      string   `json:"user"`
	PublicKey string   `json:"public_key"`
	HostKey   string   `json:"host_key,omitempty"`
	Tunnel    string   `json:"tunnel,omitempty"`
	CFDomain  string   `json:"cf_domain,omitempty"`
	Version   string   `json:"version,omitempty"`
}

// PairSession holds the state of a pairing session
type PairSession struct {
	PairID string // UUID returned by worker (lookup key)
	Token  string // Bearer token for auth
	Code   string // pairing code (encryption key only, never sent to worker)
}

// GenerateCode creates an 8-character alphanumeric pairing code
// 36^8 = ~2.8 trillion combinations (vs 900k for 6 digits)
func GenerateCode() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	code := make([]byte, 8)
	for i := range code {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		code[i] = charset[n.Int64()]
	}
	return string(code)
}

// deriveKey derives a 32-byte AES key from the code using Argon2id
// 3 iterations, 64MB memory, 1 thread — makes GPU brute-force impractical
func deriveKey(code string, salt []byte) []byte {
	return argon2.IDKey([]byte(code), salt, 3, 64*1024, 1, 32)
}

// Encrypt encrypts data with AES-GCM using the pairing code as key
// Output format: salt (16 bytes) || nonce || ciphertext
func Encrypt(data []byte, code string) (string, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}
	key := deriveKey(code, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	// Prepend salt
	result := append(salt, ciphertext...)
	return base64.StdEncoding.EncodeToString(result), nil
}

// Decrypt decrypts data with AES-GCM using the pairing code as key
func Decrypt(encoded string, code string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	if len(raw) < 16 {
		return nil, fmt.Errorf("data too short")
	}

	salt := raw[:16]
	encData := raw[16:]
	key := deriveKey(code, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(encData) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := encData[:nonceSize], encData[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// EnsureSSHKey generates an ed25519 SSH key pair if not exists
func EnsureSSHKey() (string, string, error) {
	keysDir := filepath.Join(config.HopDir(), "keys")
	os.MkdirAll(keysDir, 0700)

	privPath := filepath.Join(keysDir, "hop_ed25519")
	pubPath := privPath + ".pub"

	if _, err := os.Stat(privPath); err == nil {
		pubData, err := os.ReadFile(pubPath)
		if err != nil {
			return "", "", err
		}
		return privPath, strings.TrimSpace(string(pubData)), nil
	}

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}

	privBytes, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return "", "", err
	}

	if err := os.WriteFile(privPath, pem.EncodeToMemory(privBytes), 0600); err != nil {
		return "", "", err
	}

	sshPub, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return "", "", err
	}
	pubStr := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))

	if err := os.WriteFile(pubPath, []byte(pubStr+"\n"), 0644); err != nil {
		return "", "", err
	}

	return privPath, pubStr, nil
}

// ValidateSSHPublicKey checks that a string is a valid SSH public key with no options
func ValidateSSHPublicKey(pubKey string) error {
	// Reject newlines (multi-key injection)
	if strings.Contains(pubKey, "\n") {
		return fmt.Errorf("clé SSH invalide: contient des retours à la ligne")
	}

	// Parse the key
	_, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pubKey))
	if err != nil {
		return fmt.Errorf("clé SSH invalide: %w", err)
	}

	// Reject keys with options (command=, cert-authority, etc.)
	parts := strings.Fields(pubKey)
	if len(parts) < 2 {
		return fmt.Errorf("clé SSH invalide")
	}
	keyType := parts[0]
	validTypes := []string{"ssh-ed25519", "ssh-rsa", "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521", "sk-ssh-ed25519@openssh.com", "sk-ecdsa-sha2-nistp256@openssh.com"}
	valid := false
	for _, t := range validTypes {
		if keyType == t {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("clé SSH invalide: type inconnu ou options détectées")
	}

	return nil
}

// PublishPairData encrypts and sends pair data to the worker
// Returns a PairSession with the UUID lookup key and bearer token
func PublishPairData(code string, data *PairData) (*PairSession, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	encrypted, err := Encrypt(jsonData, code)
	if err != nil {
		return nil, err
	}

	body := fmt.Sprintf(`{"data":"%s"}`, encrypted)
	resp, err := httpClient.Post(GetWorkerURL()+"/pair", "application/json", strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("erreur connexion au serveur de pairing: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("erreur serveur: HTTP %d", resp.StatusCode)
	}

	var result struct {
		PairID string `json:"pair_id"`
		Token  string `json:"token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return nil, err
	}

	return &PairSession{
		PairID: result.PairID,
		Token:  result.Token,
		Code:   code,
	}, nil
}

// FetchPairData retrieves and decrypts pair data from the worker
func FetchPairData(pairID string, code string) (*PairData, error) {
	resp, err := httpClient.Get(GetWorkerURL() + "/pair/" + pairID)
	if err != nil {
		return nil, fmt.Errorf("erreur connexion: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("pairing non trouvé ou expiré")
	}

	var result struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return nil, err
	}

	decrypted, err := Decrypt(result.Data, code)
	if err != nil {
		return nil, fmt.Errorf("code incorrect (déchiffrement échoué)")
	}

	var pairData PairData
	if err := json.Unmarshal(decrypted, &pairData); err != nil {
		return nil, err
	}

	return &pairData, nil
}

// SendResponse sends the client's response back through the worker (requires token)
func SendResponse(session *PairSession, data *PairData) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	encrypted, err := Encrypt(jsonData, session.Code)
	if err != nil {
		return err
	}

	body := fmt.Sprintf(`{"data":"%s"}`, encrypted)
	req, err := http.NewRequest("POST", GetWorkerURL()+"/pair/"+session.PairID+"/response", strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Pair-Token", session.Token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 409 {
		return fmt.Errorf("une réponse a déjà été postée (possible attaque)")
	}
	if resp.StatusCode == 401 {
		return fmt.Errorf("token invalide")
	}

	return nil
}

// WaitForResponse polls the worker for the client's response (requires token)
func WaitForResponse(session *PairSession, timeout time.Duration) (*PairData, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest("GET", GetWorkerURL()+"/pair/"+session.PairID+"/response", nil)
		req.Header.Set("X-Pair-Token", session.Token)

		resp, err := httpClient.Do(req)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		if resp.StatusCode == 404 {
			resp.Body.Close()
			time.Sleep(2 * time.Second)
			continue
		}

		var result struct {
			Data string `json:"data"`
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("réponse invalide du serveur")
		}
		resp.Body.Close()

		decrypted, err := Decrypt(result.Data, session.Code)
		if err != nil {
			return nil, fmt.Errorf("déchiffrement échoué (possible attaque)")
		}

		var pairData PairData
		if err := json.Unmarshal(decrypted, &pairData); err != nil {
			return nil, fmt.Errorf("données de pairing corrompues")
		}
		return &pairData, nil
	}

	return nil, fmt.Errorf("timeout: pas de réponse reçue")
}

// Cleanup deletes pairing data from the worker (requires token)
func Cleanup(session *PairSession) {
	req, _ := http.NewRequest("DELETE", GetWorkerURL()+"/pair/"+session.PairID, nil)
	req.Header.Set("X-Pair-Token", session.Token)
	httpClient.Do(req)
}

// AddAuthorizedKey adds a validated public key to ~/.ssh/authorized_keys
func AddAuthorizedKey(pubKey string) error {
	// Validate the key first
	if err := ValidateSSHPublicKey(pubKey); err != nil {
		return err
	}

	sshDir := filepath.Join(os.Getenv("HOME"), ".ssh")
	os.MkdirAll(sshDir, 0700)

	authKeysPath := filepath.Join(sshDir, "authorized_keys")

	if existing, err := os.ReadFile(authKeysPath); err == nil {
		if strings.Contains(string(existing), pubKey) {
			return nil
		}
	}

	f, err := os.OpenFile(authKeysPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(pubKey + "\n")
	return err
}

// ApplyCFConfig saves the Cloudflare domain config (NOT the API key — that goes via SSH)
func ApplyCFConfig(cfDomain string) error {
	if cfDomain == "" {
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	cfg.Cloudflare = config.CloudflareConfig{
		Domain: cfDomain,
	}

	return cfg.Save()
}

// BuildCFEnvContent creates the content for the cloudflare.env file
func BuildCFEnvContent(cfEmail, cfAPIKey, cfDomain string) string {
	return fmt.Sprintf("CF_USER=%s\nCF_DOMAIN=%s\nCF_API_KEY=%s\n", cfEmail, cfDomain, cfAPIKey)
}

// LoadCFCredentials reads the CF credentials from the env file
func LoadCFCredentials() (email, apiKey string) {
	cfg, err := config.Load()
	if err != nil || cfg.Cloudflare.EnvFile == "" {
		return "", ""
	}

	path := config.ExpandPath(cfg.Cloudflare.EnvFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}

	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "CF_USER":
			email = parts[1]
		case "CF_API_KEY":
			apiKey = parts[1]
		}
	}
	return
}
