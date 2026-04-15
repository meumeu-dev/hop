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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/meumeu-dev/hop/internal/config"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/ssh"
)

const DefaultWorkerURL = "https://hop-pair.meumeudev.workers.dev"

func GetWorkerURL() string {
	configPath := config.ConfigPath()
	data, err := os.ReadFile(configPath)
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "worker_url:") {
				url := strings.TrimSpace(strings.TrimPrefix(line, "worker_url:"))
				url = strings.Trim(url, "\"'")
				if url != "" && strings.HasPrefix(url, "https://") {
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
	CFDomain  string   `json:"cf_domain,omitempty"`
	CFEnv     string   `json:"cf_env,omitempty"`
	Version   string   `json:"version,omitempty"`
}

// PairSession holds the state of a pairing session.
// Since v3 the code is both the lookup key AND the encryption secret —
// no more UUID/token split. The worker rate-limits and expires after 120s.
type PairSession struct {
	Code string // 8-char alphanumeric code
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

// PublishPairData encrypts `data` with `code` and deposits it on the worker
// under the key `code`. The code is the ONLY lookup key — the client types it
// and both sides derive the AES key from it via Argon2id.
func PublishPairData(code string, data *PairData) (*PairSession, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	encrypted, err := Encrypt(jsonData, code)
	if err != nil {
		return nil, err
	}

	bodyJSON, _ := json.Marshal(map[string]string{"code": code, "data": encrypted})
	resp, err := httpClient.Post(GetWorkerURL()+"/pair", "application/json", strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, fmt.Errorf("erreur connexion au serveur de pairing: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 409 {
		return nil, fmt.Errorf("code deja utilise, retente")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("erreur serveur: HTTP %d", resp.StatusCode)
	}
	return &PairSession{Code: code}, nil
}

// FetchPairData retrieves and decrypts pair data from the worker by code.
func FetchPairData(code string) (*PairData, error) {
	resp, err := httpClient.Get(GetWorkerURL() + "/pair/" + code)
	if err != nil {
		return nil, fmt.Errorf("erreur connexion: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("pairing non trouvé ou expiré")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("erreur serveur: HTTP %d", resp.StatusCode)
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

// SendResponse deposits the client's encrypted response under the same code.
// No token — the worker allows up to 5 responses; the server picks the first
// one that decrypts correctly.
func SendResponse(session *PairSession, data *PairData) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	encrypted, err := Encrypt(jsonData, session.Code)
	if err != nil {
		return err
	}
	bodyJSON, _ := json.Marshal(map[string]string{"data": encrypted})
	resp, err := httpClient.Post(GetWorkerURL()+"/pair/"+session.Code+"/response",
		"application/json", strings.NewReader(string(bodyJSON)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return fmt.Errorf("trop de réponses postées (DoS ?)")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("erreur serveur: HTTP %d", resp.StatusCode)
	}
	return nil
}

// WaitForResponse polls the worker and tries each of the up-to-5 response
// slots; returns the first that decrypts with the code. Fake responses from
// an attacker who doesn't know the code are silently skipped.
func WaitForResponse(session *PairSession, timeout time.Duration) (*PairData, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for idx := 0; idx < 5; idx++ {
			resp, err := httpClient.Get(GetWorkerURL() + "/pair/" + session.Code + "/response?idx=" + fmt.Sprint(idx))
			if err != nil {
				break
			}
			if resp.StatusCode == 404 {
				resp.Body.Close()
				continue
			}
			var result struct {
				Data string `json:"data"`
			}
			if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
				resp.Body.Close()
				continue
			}
			resp.Body.Close()
			decrypted, err := Decrypt(result.Data, session.Code)
			if err != nil {
				continue
			}
			var pairData PairData
			if err := json.Unmarshal(decrypted, &pairData); err != nil {
				continue
			}
			return &pairData, nil
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("timeout: pas de réponse reçue")
}

// Cleanup deletes the pairing session from the worker (best-effort).
func Cleanup(session *PairSession) {
	req, _ := http.NewRequest("DELETE", GetWorkerURL()+"/pair/"+session.Code, nil)
	httpClient.Do(req)
}

// AddAuthorizedKey adds a validated public key to ~/.ssh/authorized_keys.
// On Windows, if the current user is a local Administrator, OpenSSH
// Server ignores the per-user file and only reads
// C:\ProgramData\ssh\administrators_authorized_keys — so we write to
// BOTH locations (best-effort) and fix the strict ACLs the Windows
// sshd demands.
func AddAuthorizedKey(pubKey string) error {
	if err := ValidateSSHPublicKey(pubKey); err != nil {
		return err
	}

	home, _ := os.UserHomeDir()
	sshDir := filepath.Join(home, ".ssh")
	os.MkdirAll(sshDir, 0700)

	if err := appendKeyUnique(filepath.Join(sshDir, "authorized_keys"), pubKey); err != nil {
		return err
	}

	// Windows admin fallback — see doc comment above.
	// Only attempt the admin path if the current user is in the
	// Administrators group; otherwise administrators_authorized_keys
	// is irrelevant (sshd will use the per-user file for a standard
	// account) and we'd just trigger a UAC prompt the user can't answer.
	if runtime.GOOS == "windows" && isWindowsAdmin() {
		adminDir := `C:\ProgramData\ssh`
		adminFile := filepath.Join(adminDir, "administrators_authorized_keys")
		if _, err := os.Stat(adminDir); err == nil {
			if err := appendKeyUnique(adminFile, pubKey); err == nil {
				fixWindowsAdminACL(adminFile)
			} else if os.IsPermission(err) {
				// Elevated child via UAC
				tryElevateAndInstallKey(pubKey)
			}
		}
	}
	return nil
}

// isWindowsAdmin returns true if the current process's user is a member
// of the local Administrators group (SID S-1-5-32-544). Uses `whoami
// /groups` which is locale-independent because we match the SID, not
// the translated group name.
func isWindowsAdmin() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	out, err := exec.Command("whoami", "/groups").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "S-1-5-32-544")
}

// AppendKeyUniqueExported is exported so the hidden Windows elevated
// helper (cmd/winadmin.go) can reuse the same dedupe logic.
func AppendKeyUniqueExported(path, pubKey string) error {
	return appendKeyUnique(path, pubKey)
}

func appendKeyUnique(path, pubKey string) error {
	if existing, err := os.ReadFile(path); err == nil {
		if strings.Contains(string(existing), pubKey) {
			return nil
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(pubKey + "\n")
	return err
}

// tryElevateAndInstallKey re-launches hop.exe elevated via UAC to
// install the pub key into administrators_authorized_keys. No-op on
// non-Windows. Returns best-effort — user can still decline UAC.
func tryElevateAndInstallKey(pubKey string) {
	if runtime.GOOS != "windows" {
		return
	}
	self, err := os.Executable()
	if err != nil {
		return
	}
	arg := base64.StdEncoding.EncodeToString([]byte(pubKey))
	// PowerShell Start-Process -Verb RunAs triggers the UAC prompt.
	// -Wait so we block until the elevated hop finishes.
	ps := fmt.Sprintf(
		`Start-Process -FilePath "%s" -ArgumentList '_win-admin-authkey','%s' -Verb RunAs -Wait -WindowStyle Hidden`,
		self, arg,
	)
	exec.Command("powershell", "-NoProfile", "-Command", ps).Run()
}

// fixWindowsAdminACL runs icacls to match OpenSSH Windows' expectations
// on administrators_authorized_keys: no inheritance, only Administrators
// and SYSTEM with full control. Uses well-known SIDs so it works on any
// localized Windows (fr: "Administrateurs", de: "Administratoren", etc.)
//   S-1-5-32-544 = BUILTIN\Administrators
//   S-1-5-18     = NT AUTHORITY\SYSTEM
func fixWindowsAdminACL(path string) {
	if runtime.GOOS != "windows" {
		return
	}
	_ = exec.Command("icacls", path, "/inheritance:r",
		"/grant", "*S-1-5-32-544:F", "/grant", "*S-1-5-18:F").Run()
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
