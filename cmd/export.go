package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/meumeu-dev/hop/internal/pairing"
	"github.com/spf13/cobra"
)

var exportFile string
var exportCloud bool

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Exporte la config hop (chiffree)",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		// Include secrets
		secrets, _ := config.LoadSecrets()

		exportData := map[string]interface{}{
			"config":  cfg,
			"secrets": secrets,
			"version": version,
		}

		jsonData, err := json.Marshal(exportData)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		// Ask for password
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Mot de passe pour chiffrer le backup: ")
		password, _ := reader.ReadString('\n')
		password = strings.TrimSpace(password)
		if len(password) < 8 {
			fmt.Fprintln(os.Stderr, "Mot de passe trop court (min 8 caracteres)")
			os.Exit(1)
		}

		encrypted, err := pairing.Encrypt(jsonData, password)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur chiffrement: %v\n", err)
			os.Exit(1)
		}

		if exportCloud {
			runExportCloud(encrypted)
			return
		}

		// Default: file export
		if exportFile == "" {
			exportFile = "hop-backup.enc"
		}
		runExportFile(encrypted)
	},
}

func runExportFile(encrypted string) {
	if err := os.WriteFile(exportFile, []byte(encrypted), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("→ Config exportee dans %s\n", exportFile)
	fmt.Println("  Pour restaurer: hop import " + exportFile)
}

func runExportCloud(encrypted string) {
	workerURL := pairing.GetWorkerURL()

	bodyJSON, _ := json.Marshal(map[string]string{"data": encrypted})
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(workerURL+"/pair", "application/json", strings.NewReader(string(bodyJSON)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur upload: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "Erreur upload: HTTP %d\n", resp.StatusCode)
		os.Exit(1)
	}

	var result struct {
		PairID string `json:"pair_id"`
		Token  string `json:"token"`
	}
	json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&result)

	if result.PairID == "" {
		fmt.Fprintln(os.Stderr, "Erreur: reponse invalide du worker")
		os.Exit(1)
	}

	importToken := "cloud:" + result.PairID

	fmt.Println("→ Config uploadee (chiffree, expire dans 2 min)")
	fmt.Println()
	fmt.Println("Sur l'autre machine, lance:")
	fmt.Printf("  hop import %s\n", importToken)
	fmt.Println()
	fmt.Println("Le mot de passe sera demande a l'import.")
}

var importCmd = &cobra.Command{
	Use:   "import <fichier-ou-token>",
	Short: "Importe une config hop exportee",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		source := args[0]

		var encrypted string

		if strings.HasPrefix(source, "cloud:") {
			// Cloud import
			pairID := strings.TrimPrefix(source, "cloud:")
			if pairID == "" || strings.Contains(pairID, "/") || strings.Contains(pairID, "..") {
				fmt.Fprintln(os.Stderr, "Token invalide")
				os.Exit(1)
			}

			workerURL := pairing.GetWorkerURL()
			client := &http.Client{Timeout: 15 * time.Second}
			resp, err := client.Get(workerURL + "/pair/" + pairID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
				os.Exit(1)
			}
			defer resp.Body.Close()

			if resp.StatusCode == 404 {
				fmt.Fprintln(os.Stderr, "Backup non trouve (expire apres 2 min)")
				os.Exit(1)
			}
			if resp.StatusCode != 200 {
				fmt.Fprintf(os.Stderr, "Erreur: HTTP %d\n", resp.StatusCode)
				os.Exit(1)
			}

			var result struct {
				Data string `json:"data"`
			}
			json.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(&result)
			encrypted = result.Data
		} else {
			// File import
			data, err := os.ReadFile(source)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Erreur lecture %s: %v\n", source, err)
				os.Exit(1)
			}
			encrypted = strings.TrimSpace(string(data))
		}

		if encrypted == "" {
			fmt.Fprintln(os.Stderr, "Donnees vides")
			os.Exit(1)
		}

		// Ask for password
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Mot de passe: ")
		password, _ := reader.ReadString('\n')
		password = strings.TrimSpace(password)

		decrypted, err := pairing.Decrypt(encrypted, password)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Mot de passe incorrect ou donnees corrompues")
			os.Exit(1)
		}

		var exportData struct {
			Config  *config.Config  `json:"config"`
			Secrets *config.Secrets `json:"secrets"`
			Version string          `json:"version"`
		}
		if err := json.Unmarshal(decrypted, &exportData); err != nil {
			fmt.Fprintln(os.Stderr, "Donnees corrompues")
			os.Exit(1)
		}

		// Show what will be imported
		fmt.Println()
		if exportData.Version != "" {
			fmt.Printf("→ Backup hop %s\n", exportData.Version)
		}
		if exportData.Config != nil {
			fmt.Printf("→ %d machine(s), %d service(s), %d alias\n",
				len(exportData.Config.Machines),
				len(exportData.Config.Services),
				len(exportData.Config.Aliases))
		}

		fmt.Print("\nImporter cette config ? (ecrase la config actuelle) [o/N]: ")
		confirm, _ := reader.ReadString('\n')
		confirm = strings.TrimSpace(strings.ToLower(confirm))
		if confirm != "o" && confirm != "oui" && confirm != "y" && confirm != "yes" {
			fmt.Println("Annule.")
			return
		}

		// Save
		if exportData.Config != nil {
			if err := exportData.Config.Save(); err != nil {
				fmt.Fprintf(os.Stderr, "Erreur sauvegarde config: %v\n", err)
				os.Exit(1)
			}
		}
		if exportData.Secrets != nil {
			if err := exportData.Secrets.Save(); err != nil {
				fmt.Fprintf(os.Stderr, "Erreur sauvegarde secrets: %v\n", err)
				os.Exit(1)
			}
		}

		fmt.Println("→ Config importee avec succes.")
	},
}

func init() {
	exportCmd.Flags().StringVarP(&exportFile, "file", "f", "", "Fichier de sortie (defaut: hop-backup.enc)")
	exportCmd.Flags().BoolVar(&exportCloud, "cloud", false, "Upload chiffre sur le worker (lien temporaire 2min)")
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(importCmd)
}
