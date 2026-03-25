package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/meumeu-dev/hop/internal/dashboard"
	"github.com/spf13/cobra"
)

var remoteKey string

var remoteCmd = &cobra.Command{
	Use:   "remote",
	Short: "Gere les connexions vers d'autres hop distants",
}

var remoteAddCmd = &cobra.Command{
	Use:   "add <nom> <url>",
	Short: "Ajoute un hop distant",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		rawURL := args[1]

		if err := config.ValidateName(name); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
		if err := config.ValidateURL(rawURL); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if cfg.Remotes == nil {
			cfg.Remotes = make(map[string]config.Remote)
		}

		cfg.Remotes[name] = config.Remote{URL: rawURL}

		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		// Store key in secrets
		if remoteKey != "" {
			secrets, _ := config.LoadSecrets()
			secrets.RemoteKeys[rawURL] = remoteKey
			secrets.Save()
		}

		fmt.Printf("Remote '%s' ajouté (%s)\n", name, rawURL)
	},
}

var remoteRemoveCmd = &cobra.Command{
	Use:   "remove <nom>",
	Short: "Supprime un hop distant",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if r, ok := cfg.Remotes[name]; ok {
			// Clean up secrets
			secrets, _ := config.LoadSecrets()
			delete(secrets.RemoteKeys, r.URL)
			secrets.Save()
		}

		delete(cfg.Remotes, name)

		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Remote '%s' supprimé.\n", name)
	},
}

var remoteListCmd = &cobra.Command{
	Use:   "list",
	Short: "Liste les hop distants avec statut",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if len(cfg.Remotes) == 0 {
			fmt.Println("Aucun remote configuré.")
			return
		}

		secrets, _ := config.LoadSecrets()

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Println("Remotes:")
		for name, r := range cfg.Remotes {
			status := "offline"
			resp, err := dashboard.SafeClient.Get(r.URL + "/api/ping")
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == 200 {
					status = "online"
				}
			}

			auth := ""
			if _, ok := secrets.RemoteKeys[r.URL]; ok {
				auth = " [cle configuree]"
			}

			fmt.Fprintf(w, "  %s\t%s\t[%s]%s\n", name, r.URL, status, auth)
		}
		w.Flush()
	},
}

var remoteInfoCmd = &cobra.Command{
	Use:   "info <nom>",
	Short: "Affiche la config d'un hop distant",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		remote, ok := cfg.Remotes[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "Remote '%s' non trouvé.\n", name)
			os.Exit(1)
		}

		secrets, _ := config.LoadSecrets()

		req, _ := http.NewRequest("GET", remote.URL+"/api/config", nil)
		if key, ok := secrets.RemoteKeys[remote.URL]; ok {
			req.Header.Set("X-Hop-Key", key)
		}

		resp, err := dashboard.SafeClient.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur connexion à '%s': %v\n", name, err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		if resp.StatusCode == 401 {
			fmt.Fprintf(os.Stderr, "Accès refusé. Clé API invalide pour '%s'.\n", name)
			os.Exit(1)
		}

		body, _ := io.ReadAll(resp.Body)

		var remoteCfg config.Config
		json.Unmarshal(body, &remoteCfg)

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)

		fmt.Printf("=== %s (%s) ===\n\n", name, remote.URL)

		if len(remoteCfg.Services) > 0 {
			fmt.Println("Services:")
			for sname, s := range remoteCfg.Services {
				fmt.Fprintf(w, "  %s\t%s\n", sname, s.Desc)
			}
			w.Flush()
		}

		if len(remoteCfg.Machines) > 0 {
			fmt.Println("\nMachines:")
			for mname, m := range remoteCfg.Machines {
				fmt.Fprintf(w, "  %s\t%s@%s\n", mname, m.User, m.IP)
			}
			w.Flush()
		}
	},
}

func init() {
	remoteAddCmd.Flags().StringVar(&remoteKey, "key", "", "Clé API du hop distant")
	remoteCmd.AddCommand(remoteAddCmd)
	remoteCmd.AddCommand(remoteRemoveCmd)
	remoteCmd.AddCommand(remoteListCmd)
	remoteCmd.AddCommand(remoteInfoCmd)
	rootCmd.AddCommand(remoteCmd)
}
