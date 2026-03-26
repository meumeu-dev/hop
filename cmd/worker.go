package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/meumeu-dev/hop/internal/config"
	"github.com/meumeu-dev/hop/internal/pairing"
	"github.com/spf13/cobra"
)

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Configure le worker de pairing",
}

var workerURLCmd = &cobra.Command{
	Use:   "url [url]",
	Short: "Affiche ou configure l'URL du worker",
	Long: `Sans argument: affiche l'URL du worker actuel.
Avec une URL: configure un worker custom (ton propre relay).
Avec "default": revient au worker par defaut.

Le worker sert de relay chiffre E2E pour le pairing et l'export cloud.
Tout est chiffre cote client — le worker ne voit jamais les donnees en clair.
Le code source du worker est dans le dossier worker/ du repo.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if len(args) == 0 {
			current := pairing.GetWorkerURL()
			if cfg.WorkerURL != "" {
				fmt.Printf("Worker custom: %s\n", current)
			} else {
				fmt.Printf("Worker par defaut: %s\n", current)
			}
			fmt.Println("\nLe relay est chiffre E2E — il ne voit jamais vos donnees en clair.")
			return
		}

		url := args[0]
		if url == "default" || url == "reset" {
			cfg.WorkerURL = ""
			if err := cfg.Save(); err != nil {
				fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("→ Worker remis par defaut: %s\n", pairing.DefaultWorkerURL)
			return
		}

		if !strings.HasPrefix(url, "https://") {
			fmt.Fprintln(os.Stderr, "Erreur: l'URL doit commencer par https://")
			os.Exit(1)
		}

		cfg.WorkerURL = url
		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("→ Worker configure: %s\n", url)
		fmt.Println("  Tout le pairing et l'export cloud passeront par ce relay.")
		fmt.Println("  Le relay est chiffre E2E — il ne voit jamais vos donnees en clair.")
	},
}

func init() {
	workerCmd.AddCommand(workerURLCmd)
	rootCmd.AddCommand(workerCmd)
}
