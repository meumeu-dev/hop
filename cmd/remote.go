package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var remoteCmd = &cobra.Command{
	Use:   "remote",
	Short: "Gère les connexions vers d'autres hop distants",
}

var remoteAddCmd = &cobra.Command{
	Use:   "add <nom> <url>",
	Short: "Ajoute un hop distant",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		url := args[1]

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if cfg.Remotes == nil {
			cfg.Remotes = make(map[string]config.Remote)
		}

		cfg.Remotes[name] = config.Remote{URL: url}

		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Remote '%s' ajouté (%s)\n", name, url)
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
	Short: "Liste les hop distants",
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

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Println("Remotes:")
		for name, r := range cfg.Remotes {
			status := "?"
			resp, err := http.Get(r.URL + "/api/ping")
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == 200 {
					status = "online"
				} else {
					status = "offline"
				}
			} else {
				status = "offline"
			}
			fmt.Fprintf(w, "  %s\t%s\t[%s]\n", name, r.URL, status)
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

		resp, err := http.Get(remote.URL + "/api/config")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur connexion à '%s': %v\n", name, err)
			os.Exit(1)
		}
		defer resp.Body.Close()

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
	remoteCmd.AddCommand(remoteAddCmd)
	remoteCmd.AddCommand(remoteRemoveCmd)
	remoteCmd.AddCommand(remoteListCmd)
	remoteCmd.AddCommand(remoteInfoCmd)
	rootCmd.AddCommand(remoteCmd)
}
