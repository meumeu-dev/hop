package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/meumeu-dev/hop/internal/account"
	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Se connecter a son compte hop",
	Long: `hop login                    # interactif
hop login email@example.com  # email en argument`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := config.Init(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		reader := bufio.NewReader(os.Stdin)

		email := ""
		if len(args) > 0 {
			email = args[0]
		} else {
			fmt.Print("Email: ")
			e, _ := reader.ReadString('\n')
			email = strings.TrimSpace(e)
		}
		if email == "" {
			fmt.Fprintln(os.Stderr, "Email requis.")
			os.Exit(1)
		}

		fmt.Print("Mot de passe: ")
		passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}
		password := string(passwordBytes)
		if password == "" {
			fmt.Fprintln(os.Stderr, "Mot de passe requis.")
			os.Exit(1)
		}

		// Try login first, register if not found
		fmt.Println("→ Connexion...")
		client := account.NewClient(account.GetWorkerURL())
		session, err := client.Login(email, password)
		if err != nil {
			// Try register
			fmt.Print("Compte inexistant. Creer un compte ? [o/N]: ")
			choice, _ := reader.ReadString('\n')
			choice = strings.TrimSpace(strings.ToLower(choice))
			if choice != "o" && choice != "oui" && choice != "y" && choice != "yes" {
				os.Exit(0)
			}

			fmt.Print("Username: ")
			username, _ := reader.ReadString('\n')
			username = strings.TrimSpace(username)
			if username == "" {
				fmt.Fprintln(os.Stderr, "Username requis.")
				os.Exit(1)
			}

			fmt.Println("→ Creation du compte...")
			session, err = client.Register(email, username, password)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("→ Compte cree: %s\n", session.Username)
		}

		// Save session
		if err := account.SaveSession(session); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur sauvegarde session: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("→ Connecte en tant que %s\n", session.Username)
		fmt.Println("  hop sync pour synchroniser les machines")
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Se deconnecter",
	Run: func(cmd *cobra.Command, args []string) {
		session, err := account.LoadSession()
		if err != nil {
			fmt.Println("Pas connecte.")
			return
		}

		client := account.NewClient(account.GetWorkerURL())
		client.Logout(session.Token)
		account.DeleteSession()
		fmt.Println("→ Deconnecte.")
	},
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronise les machines avec le compte",
	Long: `hop sync        # push + pull (merge)
hop sync push   # envoie la config locale vers le cloud
hop sync pull   # recupere la config du cloud`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		session, err := account.LoadSession()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Pas connecte. Fais d'abord: hop login")
			os.Exit(1)
		}

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		client := account.NewClient(account.GetWorkerURL())
		mode := "sync"
		if len(args) > 0 {
			mode = args[0]
		}

		switch mode {
		case "push":
			runSyncPush(client, session, cfg)
		case "pull":
			runSyncPull(client, session, cfg)
		default:
			runSyncPush(client, session, cfg)
			runSyncPull(client, session, cfg)
		}
	},
}

func runSyncPush(client *account.Client, session *account.Session, cfg *config.Config) {
	fmt.Println("→ Push machines vers le cloud...")

	// Serialize machines
	machinesJSON, err := json.Marshal(cfg.Machines)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
		os.Exit(1)
	}

	// Encrypt with data key
	encrypted, err := account.EncryptData(machinesJSON, session.DataKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur chiffrement: %v\n", err)
		os.Exit(1)
	}

	if err := client.PushMachines(session.Token, encrypted); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur push: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("→ %d machines synchronisees\n", len(cfg.Machines))
}

func runSyncPull(client *account.Client, session *account.Session, cfg *config.Config) {
	fmt.Println("→ Pull machines depuis le cloud...")

	encrypted, err := client.PullMachines(session.Token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur pull: %v\n", err)
		os.Exit(1)
	}

	if encrypted == "" {
		fmt.Println("→ Aucune donnee dans le cloud.")
		return
	}

	decrypted, err := account.DecryptData(encrypted, session.DataKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erreur dechiffrement: %v\n", err)
		os.Exit(1)
	}

	var remoteMachines map[string]config.Machine
	if err := json.Unmarshal(decrypted, &remoteMachines); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur parsing: %v\n", err)
		os.Exit(1)
	}

	// Merge: add remote machines not present locally
	added := 0
	for name, machine := range remoteMachines {
		if _, exists := cfg.Machines[name]; !exists {
			cfg.Machines[name] = machine
			added++
			fmt.Printf("  + %s (%s@%s)\n", name, machine.User, machine.IP)
		}
	}

	if added > 0 {
		cfg.Save()
		fmt.Printf("→ %d nouvelles machines ajoutees\n", added)
	} else {
		fmt.Println("→ Deja a jour.")
	}
}

func init() {
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
	rootCmd.AddCommand(syncCmd)
}
