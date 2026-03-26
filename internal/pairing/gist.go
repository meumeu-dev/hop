package pairing

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// PublishGist creates a private GitHub gist with encrypted pair data.
// Returns the gist ID.
func PublishGist(code string, data *PairData) (string, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	encrypted, err := Encrypt(jsonData, code)
	if err != nil {
		return "", err
	}

	// Write encrypted data to temp file
	tmpFile, err := os.CreateTemp("", "hop-pair-*.enc")
	if err != nil {
		return "", fmt.Errorf("erreur création fichier temporaire: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(encrypted); err != nil {
		tmpFile.Close()
		return "", err
	}
	tmpFile.Close()

	// Rename temp file to hop-pair.enc for the gist filename
	gistFile := tmpFile.Name()

	// Create gist using gh CLI
	cmd := exec.Command("gh", "gist", "create", "--public=false", "--filename", "hop-pair.enc", gistFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("erreur création gist (gh installé et authentifié ?): %s", strings.TrimSpace(string(output)))
	}

	// gh gist create returns the gist URL like https://gist.github.com/<user>/<id>
	gistURL := strings.TrimSpace(string(output))
	parts := strings.Split(gistURL, "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("impossible de parser l'ID du gist: %s", gistURL)
	}
	gistID := parts[len(parts)-1]
	if gistID == "" {
		return "", fmt.Errorf("ID du gist vide")
	}

	return gistID, nil
}

// FetchGist retrieves and decrypts pair data from a GitHub gist.
func FetchGist(gistID string, code string) (*PairData, error) {
	// Fetch gist content using gh API
	cmd := exec.Command("gh", "api", fmt.Sprintf("gists/%s", gistID), "--jq", ".files[\"hop-pair.enc\"].content")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("erreur récupération gist: %s", strings.TrimSpace(string(output)))
	}

	encrypted := strings.TrimSpace(string(output))
	if encrypted == "" {
		return nil, fmt.Errorf("gist vide ou fichier hop-pair.enc introuvable")
	}

	decrypted, err := Decrypt(encrypted, code)
	if err != nil {
		return nil, fmt.Errorf("code incorrect (déchiffrement échoué)")
	}

	var pairData PairData
	if err := json.Unmarshal(decrypted, &pairData); err != nil {
		return nil, fmt.Errorf("données de pairing corrompues")
	}

	return &pairData, nil
}

// PostGistResponse adds a response file to an existing gist.
func PostGistResponse(gistID string, code string, data *PairData) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	encrypted, err := Encrypt(jsonData, code)
	if err != nil {
		return err
	}

	// Write encrypted response to temp file
	tmpFile, err := os.CreateTemp("", "hop-pair-response-*.enc")
	if err != nil {
		return fmt.Errorf("erreur création fichier temporaire: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(encrypted); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	// Update gist with response file
	cmd := exec.Command("gh", "gist", "edit", gistID, "--add", tmpFile.Name(), "--filename", "hop-pair-response.enc")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("erreur mise à jour gist: %s", strings.TrimSpace(string(output)))
	}

	return nil
}

// WaitGistResponse polls a gist for the response file.
func WaitGistResponse(gistID string, code string, timeout time.Duration) (*PairData, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cmd := exec.Command("gh", "api", fmt.Sprintf("gists/%s", gistID), "--jq", ".files[\"hop-pair-response.enc\"].content")
		output, err := cmd.CombinedOutput()
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}

		encrypted := strings.TrimSpace(string(output))
		if encrypted == "" || encrypted == "null" {
			time.Sleep(3 * time.Second)
			continue
		}

		decrypted, err := Decrypt(encrypted, code)
		if err != nil {
			return nil, fmt.Errorf("déchiffrement réponse échoué (possible attaque)")
		}

		var pairData PairData
		if err := json.Unmarshal(decrypted, &pairData); err != nil {
			return nil, fmt.Errorf("données de pairing corrompues")
		}

		return &pairData, nil
	}

	return nil, fmt.Errorf("timeout: pas de réponse reçue")
}

// CleanupGist deletes a gist.
func CleanupGist(gistID string) {
	exec.Command("gh", "gist", "delete", gistID).Run()
}
