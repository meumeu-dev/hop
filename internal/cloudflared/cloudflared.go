package cloudflared

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/meumeu-dev/hop/internal/config"
)

func BinPath() string {
	return filepath.Join(config.HopDir(), "bin", "cloudflared")
}

// Path returns the path to cloudflared, checking system then local
func Path() string {
	// Check system PATH first
	if p, err := exec.LookPath("cloudflared"); err == nil {
		return p
	}
	// Check hop local bin
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

// Install downloads cloudflared to ~/.hop/bin/
func Install() error {
	binDir := filepath.Join(config.HopDir(), "bin")
	if err := os.MkdirAll(binDir, 0700); err != nil {
		return fmt.Errorf("impossible de créer %s: %w", binDir, err)
	}

	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "amd64"
	} else if arch == "arm64" {
		arch = "arm64"
	} else if arch == "arm" {
		arch = "arm"
	}

	url := fmt.Sprintf("https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-%s", arch)
	fmt.Printf("→ Téléchargement de cloudflared (%s)...\n", arch)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("erreur téléchargement: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("erreur téléchargement: HTTP %d", resp.StatusCode)
	}

	dst := BinPath()
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("erreur écriture: %w", err)
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	if err != nil {
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
