package account

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/meumeu-dev/hop/internal/pairing"
	"golang.org/x/crypto/argon2"
)

// Session represents a logged-in account
type Session struct {
	AccountID string `json:"account_id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	Token     string `json:"token"`
	DataKey   string `json:"data_key"` // hex-encoded AES key for encrypting machine data
}

// Client handles communication with the worker
type Client struct {
	workerURL  string
	httpClient *http.Client
}

func NewClient(workerURL string) *Client {
	return &Client{
		workerURL:  workerURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func GetWorkerURL() string {
	return pairing.GetWorkerURL()
}

// deriveAuthHash creates a hash for authentication using the server-provided random salt
// Argon2id: 3 iterations, 64MB, 4 threads, 32 byte output
func deriveAuthHash(password string, saltHex string) string {
	salt, _ := hex.DecodeString(saltHex)
	key := argon2.IDKey([]byte(password), salt, 3, 64*1024, 4, 32)
	return hex.EncodeToString(key)
}

// deriveDataKey creates a key for encrypting data (never sent to server)
// Uses a deterministic salt based on email — this is acceptable because
// this key never leaves the client and is not used for authentication
func deriveDataKey(email, password string) string {
	salt := []byte("hop-data:" + strings.ToLower(email))
	key := argon2.IDKey([]byte(password), salt, 3, 64*1024, 4, 32)
	return hex.EncodeToString(key)
}

// fetchSalt gets the random salt for an email from the server (step 1 of login)
func (c *Client) fetchSalt(email string) (string, error) {
	resp, err := c.httpClient.Get(c.workerURL + "/auth/salt?email=" + email)
	if err != nil {
		return "", fmt.Errorf("connexion: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Salt  string `json:"salt"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return "", err
	}
	if result.Salt == "" {
		return "", fmt.Errorf("salt introuvable")
	}
	return result.Salt, nil
}

// Register creates a new account
func (c *Client) Register(email, username, password string) (*Session, error) {
	// For registration, we need to first register to get the salt back,
	// but the server generates the salt. So we do a 2-step:
	// 1. Send a preliminary hash with a temporary salt
	// 2. Server stores the random salt and the server-hash of our auth_hash
	// The client's auth_hash for register uses a temp salt that the server will replace.
	// Actually — simpler: register sends the hash, server stores it + its random salt.
	// On next login, client fetches the salt, recomputes the hash.
	// Problem: register and login must use the same salt!
	// Solution: client generates a random salt, sends it with registration.

	// Generate random salt client-side for this account
	saltBytes := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, saltBytes); err != nil {
		return nil, err
	}
	authSalt := hex.EncodeToString(saltBytes)

	authHash := deriveAuthHash(password, authSalt)
	dataKey := deriveDataKey(email, password)

	body, _ := json.Marshal(map[string]string{
		"email":     email,
		"username":  username,
		"auth_hash": authHash,
		"auth_salt": authSalt,
	})

	resp, err := c.httpClient.Post(c.workerURL+"/auth/register", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("connexion: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		OK        bool   `json:"ok"`
		AccountID string `json:"account_id"`
		Username  string `json:"username"`
		Token     string `json:"token"`
		Error     string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return nil, err
	}

	if !result.OK {
		return nil, fmt.Errorf("%s", result.Error)
	}

	return &Session{
		AccountID: result.AccountID,
		Username:  result.Username,
		Email:     email,
		Token:     result.Token,
		DataKey:   dataKey,
	}, nil
}

// Login authenticates with 2-step: fetch salt, then auth
func (c *Client) Login(email, password string) (*Session, error) {
	// Step 1: fetch salt
	salt, err := c.fetchSalt(email)
	if err != nil {
		return nil, err
	}

	// Step 2: compute hash with server's salt and authenticate
	authHash := deriveAuthHash(password, salt)
	dataKey := deriveDataKey(email, password)

	body, _ := json.Marshal(map[string]string{
		"email":     email,
		"auth_hash": authHash,
	})

	resp, err := c.httpClient.Post(c.workerURL+"/auth/login", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("connexion: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		OK        bool   `json:"ok"`
		AccountID string `json:"account_id"`
		Username  string `json:"username"`
		Token     string `json:"token"`
		Error     string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return nil, err
	}

	if !result.OK {
		return nil, fmt.Errorf("%s", result.Error)
	}

	return &Session{
		AccountID: result.AccountID,
		Username:  result.Username,
		Email:     email,
		Token:     result.Token,
		DataKey:   dataKey,
	}, nil
}

// Logout invalidates the session
func (c *Client) Logout(token string) {
	req, _ := http.NewRequest("POST", c.workerURL+"/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	c.httpClient.Do(req)
}

// PushMachines sends encrypted machine data to the cloud
func (c *Client) PushMachines(token string, encryptedData string) error {
	body, _ := json.Marshal(map[string]string{"data": encryptedData})
	req, err := http.NewRequest("PUT", c.workerURL+"/account/machines", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connexion: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return fmt.Errorf("session expiree, refais: hop login")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("erreur serveur: HTTP %d", resp.StatusCode)
	}
	return nil
}

// PullMachines gets encrypted machine data from the cloud
func (c *Client) PullMachines(token string) (string, error) {
	req, _ := http.NewRequest("GET", c.workerURL+"/account/machines", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("connexion: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return "", fmt.Errorf("session expiree, refais: hop login")
	}

	var result struct {
		OK       bool   `json:"ok"`
		Machines string `json:"machines"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return "", err
	}

	return result.Machines, nil
}

// EncryptData encrypts data with AES-256-GCM using the hex-encoded data key
func EncryptData(data []byte, hexKey string) (string, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return "", err
	}

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

// DecryptData decrypts data with AES-256-GCM using the hex-encoded data key
func DecryptData(encoded string, hexKey string) ([]byte, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, err
	}

	raw, err := base64.StdEncoding.DecodeString(encoded)
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
	if len(raw) < nonceSize {
		return nil, fmt.Errorf("data too short")
	}

	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// Session storage — encrypted with random salt on disk

func sessionPath() string {
	return filepath.Join(config.HopDir(), "session.enc")
}

func sessionSaltPath() string {
	return filepath.Join(config.HopDir(), "session.salt")
}

// deriveSessionKey creates a key from a random salt stored alongside the session file
// The salt makes the key non-predictable even if someone knows the hop directory
func deriveSessionKey() ([]byte, error) {
	saltPath := sessionSaltPath()
	salt, err := os.ReadFile(saltPath)
	if err != nil {
		// Generate new random salt
		salt = make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			return nil, err
		}
		if err := os.WriteFile(saltPath, salt, 0600); err != nil {
			return nil, err
		}
	}

	// Derive key from salt + a machine-specific element (hop dir path)
	material := append(salt, []byte(config.HopDir())...)
	key := argon2.IDKey(material, []byte("hop-session-local"), 1, 16*1024, 1, 32)
	return key, nil
}

func SaveSession(s *Session) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}

	key, err := deriveSessionKey()
	if err != nil {
		return err
	}

	encrypted, err := EncryptData(data, hex.EncodeToString(key))
	if err != nil {
		return err
	}
	return os.WriteFile(sessionPath(), []byte(encrypted), 0600)
}

func LoadSession() (*Session, error) {
	data, err := os.ReadFile(sessionPath())
	if err != nil {
		return nil, err
	}

	key, err := deriveSessionKey()
	if err != nil {
		return nil, err
	}

	decrypted, err := DecryptData(string(data), hex.EncodeToString(key))
	if err != nil {
		return nil, err
	}

	var s Session
	if err := json.Unmarshal(decrypted, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func DeleteSession() {
	os.Remove(sessionPath())
	os.Remove(sessionSaltPath())
}
