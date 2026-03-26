package cloudflared

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/meumeu-dev/hop/internal/config"
)

func BinPath() string {
	return filepath.Join(config.HopDir(), "bin", "cloudflared")
}

// Path returns the path to cloudflared, checking system then local
func Path() string {
	if p, err := exec.LookPath("cloudflared"); err == nil {
		return p
	}
	local := BinPath()
	if _, err := os.Stat(local); err == nil {
		return local
	}
	return ""
}

// IsInstalled returns true if cloudflared is available
func IsInstalled() bool {
	return Path() != ""
}

// Install downloads cloudflared to ~/.hop/bin/ with checksum verification
func Install() error {
	binDir := filepath.Join(config.HopDir(), "bin")
	if err := os.MkdirAll(binDir, 0700); err != nil {
		return fmt.Errorf("impossible de créer %s: %w", binDir, err)
	}

	arch := runtime.GOARCH

	binaryName := fmt.Sprintf("cloudflared-linux-%s", arch)
	binaryURL := fmt.Sprintf("https://github.com/cloudflare/cloudflared/releases/latest/download/%s", binaryName)
	checksumURL := fmt.Sprintf("https://github.com/cloudflare/cloudflared/releases/latest/download/%s.sha256", binaryName)

	fmt.Printf("→ Téléchargement de cloudflared (%s)...\n", arch)

	// Download binary
	httpClient := &http.Client{Timeout: 120 * time.Second}
	resp, err := httpClient.Get(binaryURL)
	if err != nil {
		return fmt.Errorf("erreur téléchargement: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("erreur téléchargement: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 200<<20))
	if err != nil {
		return fmt.Errorf("erreur téléchargement: %w", err)
	}

	// Try to verify checksum
	checksumResp, err := http.Get(checksumURL)
	if err == nil && checksumResp.StatusCode == 200 {
		checksumData, err := io.ReadAll(io.LimitReader(checksumResp.Body, 1024))
		checksumResp.Body.Close()
		if err == nil {
			fields := strings.Fields(string(checksumData))
			if len(fields) > 0 {
				expected := fields[0]
				h := sha256.Sum256(data)
				actual := hex.EncodeToString(h[:])
				if expected != actual {
					return fmt.Errorf("checksum cloudflared invalide (attendu %s, obtenu %s)", expected[:16]+"...", actual[:16]+"...")
				}
				fmt.Println("→ Checksum SHA256 vérifié")
			}
		}
	}

	dst := BinPath()
	if err := os.WriteFile(dst, data, 0755); err != nil {
		return fmt.Errorf("erreur écriture: %w", err)
	}

	fmt.Println("→ cloudflared installé dans ~/.hop/bin/")
	return nil
}

// EnsureInstalled installs cloudflared if not present, returns the path
func EnsureInstalled() (string, error) {
	if p := Path(); p != "" {
		return p, nil
	}

	fmt.Println("→ cloudflared non trouvé, installation automatique...")
	if err := Install(); err != nil {
		return "", err
	}
	return BinPath(), nil
}

// Run executes a cloudflared command
func Run(args ...string) error {
	p, err := EnsureInstalled()
	if err != nil {
		return err
	}
	cmd := exec.Command(p, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
