package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var updateForce bool

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Met a jour hop vers la derniere version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("hop %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)

		fmt.Println("→ Verification de la derniere version...")
		latest, changelog, err := fetchLatestRelease()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if latest == version || latest == "v"+version {
			fmt.Println("Deja a jour.")
			return
		}

		fmt.Printf("\n→ Nouvelle version disponible: %s -> %s\n", version, latest)

		// Show changelog
		if changelog != "" {
			fmt.Println()
			fmt.Println("Changelog:")
			fmt.Println(formatChangelog(changelog))
		}
		fmt.Printf("\nhttps://github.com/meumeu-dev/hop/releases/tag/%s\n", latest)

		// Ask confirmation unless -y
		if !updateForce {
			reader := bufio.NewReader(os.Stdin)
			fmt.Print("\nMettre a jour ? [o/N]: ")
			confirm, _ := reader.ReadString('\n')
			confirm = strings.TrimSpace(strings.ToLower(confirm))
			if confirm != "o" && confirm != "oui" && confirm != "y" && confirm != "yes" {
				fmt.Println("Annule.")
				return
			}
		}

		fmt.Println()

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

		assetURL := findAssetURL(body, binaryName)
		if assetURL == "" {
			fmt.Fprintf(os.Stderr, "Erreur: binaire %s non trouve dans la release %s\n", binaryName, latest)
			os.Exit(1)
		}

		data, err := downloadAsset(assetURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur telechargement: %v\n", err)
			os.Exit(1)
		}

		if len(data) < 1024 {
			fmt.Fprintf(os.Stderr, "Erreur: fichier telecharge trop petit (%d bytes)\n", len(data))
			os.Exit(1)
		}

		execPath, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: impossible de trouver le binaire actuel: %v\n", err)
			os.Exit(1)
		}

		tmpPath := execPath + ".new"
		if err := os.WriteFile(tmpPath, data, 0755); err != nil {
			if os.IsPermission(err) {
				tmpPath = "/tmp/hop.new"
				if err := os.WriteFile(tmpPath, data, 0755); err != nil {
					fmt.Fprintf(os.Stderr, "Erreur ecriture: %v\n", err)
					os.Exit(1)
				}
				fmt.Println("→ Installation (sudo)...")
				mvCmd := exec.Command("sudo", "mv", tmpPath, execPath)
				mvCmd.Stdin = os.Stdin
				mvCmd.Stdout = os.Stdout
				mvCmd.Stderr = os.Stderr
				if err := mvCmd.Run(); err != nil {
					os.Remove(tmpPath)
					fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
					os.Exit(1)
				}
				fmt.Printf("→ hop mis a jour en %s\n", latest)
				return
			}
			fmt.Fprintf(os.Stderr, "Erreur ecriture: %v\n", err)
			os.Exit(1)
		}

		oldPath := execPath + ".old"
		os.Remove(oldPath)

		if err := os.Rename(execPath, oldPath); err != nil {
			os.Remove(tmpPath)
			if os.IsPermission(err) {
				fmt.Println("→ Installation (sudo)...")
				mvCmd := exec.Command("sudo", "bash", "-c", fmt.Sprintf("mv %s %s.old 2>/dev/null; mv %s %s", execPath, execPath, tmpPath, execPath))
				mvCmd.Stdin = os.Stdin
				mvCmd.Stdout = os.Stdout
				mvCmd.Stderr = os.Stderr
				if err := mvCmd.Run(); err != nil {
					fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
					os.Exit(1)
				}
				fmt.Printf("→ hop mis a jour en %s\n", latest)
				return
			}
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if err := os.Rename(tmpPath, execPath); err != nil {
			os.Rename(oldPath, execPath)
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		os.Remove(oldPath)
		fmt.Printf("→ hop mis a jour en %s\n", latest)
	},
}

// fetchLatestRelease returns version, changelog body, error
func fetchLatestRelease() (string, string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := githubRequest("https://api.github.com/repos/meumeu-dev/hop/releases/latest")
	if err != nil {
		return "", "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("GitHub API: %s", resp.Status)
	}

	var release struct {
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", "", err
	}
	return strings.TrimSpace(release.TagName), release.Body, nil
}

// fetchLatestVersion kept for version --check compatibility
func fetchLatestVersion() (string, error) {
	v, _, err := fetchLatestRelease()
	return v, err
}

func formatChangelog(body string) string {
	lines := strings.Split(body, "\n")
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, "  "+line)
	}
	if len(out) > 20 {
		out = out[:20]
		out = append(out, "  ...")
	}
	return strings.Join(out, "\n")
}

// CheckUpdateBackground checks for updates silently, prints a notice if available.
// Called from PersistentPreRun, runs only once per day.
func CheckUpdateBackground() {
	if version == "dev" {
		return
	}

	// Check once per day max
	markerPath := config.HopDir() + "/.last-update-check"
	if info, err := os.Stat(markerPath); err == nil {
		if time.Since(info.ModTime()) < 24*time.Hour {
			return
		}
	}

	// Touch the marker
	os.MkdirAll(config.HopDir(), 0700)
	os.WriteFile(markerPath, []byte(time.Now().Format(time.RFC3339)), 0600)

	// Quick check (2s timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	req, err := githubRequest("https://api.github.com/repos/meumeu-dev/hop/releases/latest")
	if err != nil {
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	json.NewDecoder(resp.Body).Decode(&release)
	latest := strings.TrimSpace(release.TagName)

	if latest != "" && latest != version && latest != "v"+version {
		fmt.Fprintf(os.Stderr, "\n→ Mise a jour disponible: %s -> %s (hop update)\n\n", version, latest)
	}
}

// githubToken returns a GitHub token from env or gh CLI
func githubToken() string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	if t := os.Getenv("GH_TOKEN"); t != "" {
		return t
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

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

func findAssetURL(releaseJSON []byte, assetName string) string {
	s := string(releaseJSON)
	needle := `"name":"` + assetName + `"`
	idx := strings.Index(s, needle)
	if idx < 0 {
		return ""
	}
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
	return io.ReadAll(io.LimitReader(resp.Body, 100<<20))
}

func init() {
	updateCmd.Flags().BoolVarP(&updateForce, "yes", "y", false, "Skip la confirmation")
	rootCmd.AddCommand(updateCmd)
}
