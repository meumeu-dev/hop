package tunnel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/meumeu-dev/hop/internal/pairing"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

// machineToken returns a deterministic token for this machine based on SSH key
func machineToken() string {
	pubPath := config.HopDir() + "/keys/hop_ed25519.pub"
	data, err := os.ReadFile(pubPath)
	if err != nil {
		// Fallback to hostname
		hostname, _ := os.Hostname()
		h := sha256.Sum256([]byte("hop-tunnel:" + hostname))
		return hex.EncodeToString(h[:])
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// MachineID returns a short identifier for this machine
func MachineID() string {
	hostname, _ := os.Hostname()
	return hostname
}

// Register publishes the current tunnel URL to the worker
func Register(tunnelURL string) error {
	workerURL := pairing.GetWorkerURL()
	machineID := MachineID()
	token := machineToken()

	bodyData, _ := json.Marshal(map[string]string{"url": tunnelURL, "token": token})
	body := string(bodyData)
	resp, err := httpClient.Post(workerURL+"/tunnel/"+machineID, "application/json", strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("erreur enregistrement tunnel: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("erreur enregistrement tunnel: HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// Resolve fetches the current tunnel URL for a machine from the worker
func Resolve(machineID string) (string, error) {
	workerURL := pairing.GetWorkerURL()

	resp, err := httpClient.Get(workerURL + "/tunnel/" + machineID)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", nil // no tunnel registered
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var result struct {
		URL     string `json:"url"`
		Updated int64  `json:"updated"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&result); err != nil {
		return "", err
	}

	// Check if the registration is recent (< 2 hours)
	if time.Now().UnixMilli()-result.Updated > 2*60*60*1000 {
		return "", nil // stale
	}

	return result.URL, nil
}
