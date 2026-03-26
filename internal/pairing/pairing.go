package pairing

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
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
	"golang.org/x/crypto/ssh"
)

const WorkerURL = "https://hop-pair.meumeudev.workers.dev"

// PairData is what gets encrypted and sent through the worker
type PairData struct {
	Hostname  string `json:"hostname"`
	IP        string `json:"ip,omitempty"`
	User      string `json:"user"`
	PublicKey string `json:"public_key"`
	Tunnel    string `json:"tunnel,omitempty"`
	// Cloudflare config (sent from main PC to new machine)
	CFDomain  string `json:"cf_domain,omitempty"`
	CFEmail   string `json:"cf_email,omitempty"`
	CFAPIKey  string `json:"cf_api_key,omitempty"`
}

// GenerateCode creates a 6-digit pairing code
func GenerateCode() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(900000))
	return fmt.Sprintf("%06d", n.Int64()+100000)
}

// deriveKey derives a 32-byte AES key from the 6-digit code
func deriveKey(code string) []byte {
	// Use SHA-256 of the code as the encryption key
	hash := sha256.Sum256([]byte("hop-pair-" + code))
	return hash[:]
}

// Encrypt encrypts data with AES-GCM using the pairing code as key
func Encrypt(data []byte, code string) (string, error) {
	key := deriveKey(code)
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
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts data with AES-GCM using the pairing code as key
func Decrypt(encoded string, code string) ([]byte, error) {
	key := deriveKey(code)
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// EnsureSSHKey generates an ed25519 SSH key pair if not exists
func EnsureSSHKey() (string, string, error) {
	keysDir := filepath.Join(config.HopDir(), "keys")
	os.MkdirAll(keysDir, 0700)

	privPath := filepath.Join(keysDir, "hop_ed25519")
	pubPath := privPath + ".pub"

	// Check if already exists
	if _, err := os.Stat(privPath); err == nil {
		pubData, err := os.ReadFile(pubPath)
		if err != nil {
			return "", "", err
		}
		return privPath, strings.TrimSpace(string(pubData)), nil
	}

	// Generate new key pair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}

	// Marshal private key to PEM
	privBytes, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return "", "", err
	}

	if err := os.WriteFile(privPath, pem.EncodeToMemory(privBytes), 0600); err != nil {
		return "", "", err
	}

	// Marshal public key to authorized_keys format
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

// PublishPairData encrypts and sends pair data to the worker
func PublishPairData(code string, data *PairData) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	encrypted, err := Encrypt(jsonData, code)
	if err != nil {
		return err
	}

	body := fmt.Sprintf(`{"code":"%s","data":"%s"}`, code, encrypted)
	resp, err := http.Post(WorkerURL+"/pair", "application/json", strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("erreur connexion au serveur de pairing: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("erreur serveur: HTTP %d", resp.StatusCode)
	}

	return nil
}

// FetchPairData retrieves and decrypts pair data from the worker
func FetchPairData(code string) (*PairData, error) {
	resp, err := http.Get(WorkerURL + "/pair/" + code)
	if err != nil {
		return nil, fmt.Errorf("erreur connexion: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("code invalide ou expiré")
	}

	var result struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
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

// SendResponse sends the PC's response (public key) back through the worker
func SendResponse(code string, data *PairData) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	encrypted, err := Encrypt(jsonData, code)
	if err != nil {
		return err
	}

	body := fmt.Sprintf(`{"data":"%s"}`, encrypted)
	resp, err := http.Post(WorkerURL+"/pair/"+code+"/response", "application/json", strings.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// WaitForResponse polls the worker for the PC's response
func WaitForResponse(code string, timeout time.Duration) (*PairData, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(WorkerURL + "/pair/" + code + "/response")
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
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		decrypted, err := Decrypt(result.Data, code)
		if err != nil {
			return nil, fmt.Errorf("déchiffrement échoué")
		}

		var pairData PairData
		json.Unmarshal(decrypted, &pairData)
		return &pairData, nil
	}

	return nil, fmt.Errorf("timeout: pas de réponse reçue")
}

// Cleanup deletes pairing data from the worker
func Cleanup(code string) {
	req, _ := http.NewRequest("DELETE", WorkerURL+"/pair/"+code, nil)
	http.DefaultClient.Do(req)
}

// AddAuthorizedKey adds a public key to ~/.ssh/authorized_keys
func AddAuthorizedKey(pubKey string) error {
	sshDir := filepath.Join(os.Getenv("HOME"), ".ssh")
	os.MkdirAll(sshDir, 0700)

	authKeysPath := filepath.Join(sshDir, "authorized_keys")

	// Check if key already exists
	if existing, err := os.ReadFile(authKeysPath); err == nil {
		if strings.Contains(string(existing), pubKey) {
			return nil // Already added
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

// ApplyCFConfig saves the Cloudflare config received from pairing
func ApplyCFConfig(cfDomain, cfEmail, cfAPIKey string) error {
	if cfDomain == "" {
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Write env file
	envPath := filepath.Join(config.HopDir(), "cloudflare.env")
	envContent := fmt.Sprintf("CF_USER=%s\nCF_DOMAIN=%s\nCF_API_KEY=%s\n", cfEmail, cfDomain, cfAPIKey)
	if err := os.WriteFile(envPath, []byte(envContent), 0600); err != nil {
		return err
	}

	cfg.Cloudflare = config.CloudflareConfig{
		Domain:  cfDomain,
		EnvFile: "~/.hop/cloudflare.env",
	}

	return cfg.Save()
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
