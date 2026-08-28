package cmd

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	cf "github.com/meumeu-dev/hop/internal/cloudflared"
	"github.com/meumeu-dev/hop/internal/config"
	"github.com/spf13/cobra"
)

var unlockCmd = &cobra.Command{
	Use:   "unlock [machine]",
	Short: "Deverrouille le disque chiffre d'une machine a distance",
	Long: `Ouvre une session vers dropbear dans l'initramfs d'une machine chiffree,
a travers son tunnel Cloudflare, pour saisir la passphrase LUKS.

La machine doit avoir ete preparee cote serveur (tunnel + dropbear + cle SSH
autorisee avec command="/usr/bin/cryptroot-unlock").`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		if len(cfg.Unlock) == 0 {
			fmt.Println("Aucune machine a deverrouiller configuree.")
			fmt.Println("Ajoute-en une dans ~/.hop/config.yml, section 'unlock':")
			fmt.Println()
			fmt.Println("unlock:")
			fmt.Println("  - name: mon-serveur")
			fmt.Println("    hostname: unlock-mon-serveur.exemple.com")
			fmt.Println("    token_id: xxxxx.access")
			fmt.Println("    token_secret: xxxxx")
			fmt.Println("    key_file: ~/.ssh/hop_unlock_ed25519")
			os.Exit(1)
		}

		var target *config.UnlockTarget
		if len(args) == 0 {
			if len(cfg.Unlock) == 1 {
				target = &cfg.Unlock[0]
			} else {
				fmt.Println("Plusieurs machines configurees, precise laquelle :")
				for _, t := range cfg.Unlock {
					fmt.Printf("  hop unlock %s\n", t.Name)
				}
				os.Exit(1)
			}
		} else {
			for i := range cfg.Unlock {
				if cfg.Unlock[i].Name == args[0] {
					target = &cfg.Unlock[i]
					break
				}
			}
			if target == nil {
				fmt.Fprintf(os.Stderr, "Machine '%s' inconnue dans la section 'unlock'\n", args[0])
				os.Exit(1)
			}
		}

		cfPath, err := cf.EnsureInstalled()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur cloudflared: %v\n", err)
			os.Exit(1)
		}

		proxyCmd := fmt.Sprintf("%s access ssh --hostname %%h", cfPath)
		if target.TokenID != "" && target.TokenSecret != "" {
			proxyCmd += fmt.Sprintf(" --service-token-id %s --service-token-secret %s",
				target.TokenID, target.TokenSecret)
		}

		sshArgs := []string{
			"-o", "ProxyCommand=" + proxyCmd,
			"-o", "LogLevel=ERROR",
		}

		// Epinglage de la cle hote quand elle est connue. Sans lui, quiconque
		// obtient le secret du tunnel (lisible sur /boot non chiffre si la
		// machine est volee) peut se faire passer pour elle et recuperer la
		// passphrase que tu tapes.
		if target.HostKey != "" {
			known, err := os.CreateTemp("", "hop-known-hosts-*")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Erreur fichier temporaire: %v\n", err)
				os.Exit(1)
			}
			defer os.Remove(known.Name())
			fmt.Fprintf(known, "%s %s\n", target.Hostname, target.HostKey)
			known.Close()
			sshArgs = append(sshArgs,
				"-o", "StrictHostKeyChecking=yes",
				"-o", "UserKnownHostsFile="+known.Name())
		} else {
			fmt.Fprintln(os.Stderr, "⚠ Aucune cle hote epinglee (host_key absent de la config) :")
			fmt.Fprintln(os.Stderr, "  la machine n'est pas authentifiee, ta passphrase pourrait etre")
			fmt.Fprintln(os.Stderr, "  interceptee. Relance `hop unlock setup` pour recuperer host_key.")
			sshArgs = append(sshArgs,
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null")
		}
		if target.KeyFile != "" {
			sshArgs = append(sshArgs, "-i", config.ExpandPath(target.KeyFile), "-o", "IdentitiesOnly=yes")
		}
		sshArgs = append(sshArgs, "root@"+target.Hostname)

		fmt.Printf("Connexion a %s (%s)...\n", target.Name, target.Hostname)
		c := exec.Command("ssh", sshArgs...)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			// Sortie non nulle attendue si la passphrase est refusee
			os.Exit(1)
		}
	},
}

// Commande executee par la machine elle-meme depuis son initramfs : previent
// que le disque attend une passphrase. Masquee du help (usage interne).
var unlockNotifyCmd = &cobra.Command{
	Use:    "notify",
	Short:  "Signale que cette machine attend un deverrouillage (usage initramfs)",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		dir := "/etc/hop/unlock"
		machineID := readTrimmed(dir + "/machine")
		if machineID == "" {
			machineID, _ = os.Hostname()
		}

		// Notification ntfy — envoyee depuis la machine elle-meme (le quota
		// gratuit ntfy.sh est par IP source, et les IP de sortie des Workers
		// Cloudflare sont saturees : HTTP 429 systematique depuis la-bas).
		if ntfyURL := readTrimmed(dir + "/ntfy.url"); ntfyURL != "" {
			body := strings.NewReader(machineID + " attend la passphrase de son disque chiffre.")
			if req, err := http.NewRequest("POST", ntfyURL, body); err == nil {
				req.Header.Set("Title", machineID+" attend un deverrouillage")
				req.Header.Set("Priority", "high")
				req.Header.Set("Tags", "lock")
				req.Header.Set("User-Agent", "hop-unlock-notify/1.0")
				if resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req); err == nil {
					resp.Body.Close()
					fmt.Printf("hop: ntfy HTTP %d\n", resp.StatusCode)
				} else {
					fmt.Fprintf(os.Stderr, "hop: ntfy echoue: %v\n", err)
				}
			}
		}

		// Etat cote Worker, signe HMAC (anti-spam/anti-rejeu)
		workerURL := readTrimmed(dir + "/worker.url")
		keyHex := readTrimmed(dir + "/hmac.key")
		if workerURL == "" || keyHex == "" {
			return
		}
		key, err := hex.DecodeString(keyHex)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hop: cle HMAC invalide\n")
			return
		}

		nonceBytes := make([]byte, 16)
		if _, err := rand.Read(nonceBytes); err != nil {
			return
		}
		nonce := hex.EncodeToString(nonceBytes)
		ts := time.Now().Unix()
		mac := hmac.New(sha256.New, key)
		fmt.Fprintf(mac, "%s:%s:%d", machineID, nonce, ts)

		payload, _ := json.Marshal(map[string]interface{}{
			"machine_id": machineID,
			"nonce":      nonce,
			"timestamp":  ts,
			"signature":  hex.EncodeToString(mac.Sum(nil)),
		})

		req, err := http.NewRequest("POST", workerURL+"/unlock/trigger", bytes.NewReader(payload))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		// workers.dev rejette le User-Agent par defaut de net/http (403 code 1010)
		req.Header.Set("User-Agent", "hop-unlock-notify/1.0")

		resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hop: worker injoignable: %v\n", err)
			return
		}
		defer resp.Body.Close()
		fmt.Printf("hop: worker HTTP %d\n", resp.StatusCode)
	},
}

func readTrimmed(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func init() {
	unlockCmd.AddCommand(unlockNotifyCmd)
	rootCmd.AddCommand(unlockCmd)
}
