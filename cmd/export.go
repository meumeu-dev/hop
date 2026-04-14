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

		exportData := map[string]interface{}{
			"config":  cfg,
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

	// v3: the worker endpoint is /pair and expects a {code, data} body.
	// The code doubles as the lookup key and the import token — the user
	// retypes it on the other machine.
	code := pairing.GenerateCode()
	bodyJSON, _ := json.Marshal(map[string]string{"code": code, "data": encrypted})
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(workerURL+"/pair", "application/json", strings.NewReader(string(bodyJSON)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur upload: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 409 {
		fmt.Fprintln(os.Stderr, "Erreur: code deja utilise, retente")
		os.Exit(1)
	}
	if resp.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "Erreur upload: HTTP %d\n", resp.StatusCode)
		os.Exit(1)
	}

	importToken := "cloud:" + code

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
			// Cloud import — v3: code is 8 alphanumeric chars
			code := strings.ToLower(strings.TrimPrefix(source, "cloud:"))
			validCode := len(code) == 8
			for _, c := range code {
				if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
					validCode = false
					break
				}
			}
			if !validCode {
				fmt.Fprintln(os.Stderr, "Token invalide (attendu: cloud:<8-chars>)")
				os.Exit(1)
			}

			workerURL := pairing.GetWorkerURL()
			client := &http.Client{Timeout: 15 * time.Second}
			resp, err := client.Get(workerURL + "/pair/" + code)
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
			Config  *config.Config `json:"config"`
			Version string         `json:"version"`
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
		fmt.Println("→ Config importee avec succes.")
	},
}

func init() {
	exportCmd.Flags().StringVarP(&exportFile, "file", "f", "", "Fichier de sortie (defaut: hop-backup.enc)")
	exportCmd.Flags().BoolVar(&exportCloud, "cloud", false, "Upload chiffre sur le worker (lien temporaire 2min)")
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(importCmd)
}
