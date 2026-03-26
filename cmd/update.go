package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Met a jour hop vers la derniere version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("hop %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)

		fmt.Println("→ Verification de la derniere version...")
		latest, err := fetchLatestVersion()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if latest == version || latest == "v"+version {
			fmt.Println("Deja a jour.")
			return
		}

		fmt.Printf("→ Mise a jour %s -> %s\n", version, latest)

		arch := runtime.GOARCH
		binaryName := fmt.Sprintf("hop-linux-%s", arch)
		if arch == "arm" {
			binaryName = "hop-linux-arm32"
		}

		fmt.Printf("→ Telechargement %s...\n", binaryName)

		url := fmt.Sprintf("https://api.github.com/repos/meumeu-dev/hop/releases/tags/%s", latest)
		body, err := githubGet(url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		// Find asset download URL from release assets
		assetURL := findAssetURL(body, binaryName)
		if assetURL == "" {
			fmt.Fprintf(os.Stderr, "Erreur: binaire %s non trouve dans la release %s\n", binaryName, latest)
			os.Exit(1)
		}

		// Download the asset
		data, err := downloadAsset(assetURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur telechargement: %v\n", err)
			os.Exit(1)
		}

		if len(data) < 1024 {
			fmt.Fprintf(os.Stderr, "Erreur: fichier telecharge trop petit (%d bytes)\n", len(data))
			os.Exit(1)
		}

		// Get current binary path
		execPath, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: impossible de trouver le binaire actuel: %v\n", err)
			os.Exit(1)
		}

		// Atomic replace
		tmpPath := execPath + ".new"
		if err := os.WriteFile(tmpPath, data, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur ecriture: %v\n", err)
			os.Exit(1)
		}

		oldPath := execPath + ".old"
		os.Remove(oldPath)

		if err := os.Rename(execPath, oldPath); err != nil {
			os.Remove(tmpPath)
			fmt.Fprintf(os.Stderr, "Erreur: impossible de remplacer le binaire: %v\n", err)
			fmt.Fprintln(os.Stderr, "Essaie avec sudo: sudo hop update")
			os.Exit(1)
		}

		if err := os.Rename(tmpPath, execPath); err != nil {
			os.Rename(oldPath, execPath)
			fmt.Fprintf(os.Stderr, "Erreur: impossible de remplacer le binaire: %v\n", err)
			os.Exit(1)
		}

		os.Remove(oldPath)
		fmt.Printf("→ hop mis a jour en %s\n", latest)
	},
}

// githubToken returns a GitHub token from env or gh CLI
func githubToken() string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	if t := os.Getenv("GH_TOKEN"); t != "" {
		return t
	}
	// Try gh auth token
	out, err := exec.Command("gh", "auth", "token").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

// githubRequest creates an authenticated GitHub API request
func githubRequest(url string) (*http.Request, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

// githubGet fetches a GitHub API URL with auth
func githubGet(url string) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := githubRequest(url)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 10<<20))
}

// findAssetURL extracts the browser_download_url for the given asset name
func findAssetURL(releaseJSON []byte, assetName string) string {
	// Simple string search — avoids pulling in encoding/json for a simple lookup
	s := string(releaseJSON)
	needle := `"name":"` + assetName + `"`
	idx := strings.Index(s, needle)
	if idx < 0 {
		return ""
	}
	// Find browser_download_url near this asset
	sub := s[idx:]
	urlNeedle := `"browser_download_url":"`
	uidx := strings.Index(sub, urlNeedle)
	if uidx < 0 {
		return ""
	}
	start := uidx + len(urlNeedle)
	end := strings.Index(sub[start:], `"`)
	if end < 0 {
		return ""
	}
	return sub[start : start+end]
}

// downloadAsset downloads a GitHub release asset with auth
func downloadAsset(url string) ([]byte, error) {
	client := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 100<<20)) // 100MB max
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
