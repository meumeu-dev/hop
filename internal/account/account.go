package account

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
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

// deriveAuthHash creates a hash for authentication (sent to server)
// Uses Argon2id with "auth" salt — the server hashes this again with SHA256
func deriveAuthHash(email, password string) string {
	salt := []byte("hop-auth:" + strings.ToLower(email))
	key := argon2.IDKey([]byte(password), salt, 3, 64*1024, 1, 32)
	return hex.EncodeToString(key)
}

// deriveDataKey creates a key for encrypting data (never sent to server)
// Uses Argon2id with "data" salt — completely separate from auth hash
func deriveDataKey(email, password string) string {
	salt := []byte("hop-data:" + strings.ToLower(email))
	key := argon2.IDKey([]byte(password), salt, 3, 64*1024, 1, 32)
	return hex.EncodeToString(key)
}

// Register creates a new account
func (c *Client) Register(email, username, password string) (*Session, error) {
	authHash := deriveAuthHash(email, password)
	dataKey := deriveDataKey(email, password)

	body, _ := json.Marshal(map[string]string{
		"email":     email,
		"username":  username,
		"auth_hash": authHash,
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

// Login authenticates an existing account
func (c *Client) Login(email, password string) (*Session, error) {
	authHash := deriveAuthHash(email, password)
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

// Session storage

func sessionPath() string {
	return filepath.Join(config.HopDir(), "session.json")
}

func SaveSession(s *Session) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	// Encrypt session file with a machine-specific key (SHA256 of hop dir path)
	h := sha256.Sum256([]byte(config.HopDir() + "hop-session-key"))
	encrypted, err := EncryptData(data, hex.EncodeToString(h[:]))
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

	h := sha256.Sum256([]byte(config.HopDir() + "hop-session-key"))
	decrypted, err := DecryptData(string(data), hex.EncodeToString(h[:]))
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
}
