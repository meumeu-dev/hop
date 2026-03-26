package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Version is set from main via SetVersion
var version = "dev"

func SetVersion(v string) {
	version = v
}

func GetVersion() string {
	return version
}

var checkUpdate bool

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Affiche la version de hop",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("hop %s\n", version)

		if checkUpdate {
			latest, err := fetchLatestVersion()
			if err != nil {
				fmt.Printf("Impossible de verifier les mises a jour: %v\n", err)
				return
			}
			if latest != version && latest != "v"+version {
				fmt.Printf("Nouvelle version disponible: %s\n", latest)
				fmt.Println("https://github.com/meumeu-dev/hop/releases/latest")
			} else {
				fmt.Println("Vous etes a jour.")
			}
		}
	},
}

func fetchLatestVersion() (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := githubRequest("https://api.github.com/repos/meumeu-dev/hop/releases/latest")
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GitHub API: %s", resp.Status)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return strings.TrimSpace(release.TagName), nil
}

func init() {
	versionCmd.Flags().BoolVar(&checkUpdate, "check", false, "Verifie si une mise a jour est disponible")
	rootCmd.AddCommand(versionCmd)
}
