package cmd

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/meumeu-dev/hop/internal/cfaccess"
	cf "github.com/meumeu-dev/hop/internal/cloudflared"
	"github.com/meumeu-dev/hop/internal/config"
	initrd "github.com/meumeu-dev/hop/internal/initramfs"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
)

const unlockDir = "/etc/hop/unlock"

var (
	setupMachineName string
	setupNtfyTopic   string
	setupWorkerURL   string
	setupSkipInitrd  bool
	setupForce       bool
	setupRegenHMAC   bool
	setupRegenKey    bool
)

var unlockSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Prepare CETTE machine pour le deverrouillage a distance",
	Long: `Configure de bout en bout le deverrouillage LUKS a distance de cette machine :

  1. cree (ou reutilise) un tunnel Cloudflare et sa route DNS
  2. cree l'application Cloudflare Access + le service token (policy non_identity)
  3. genere une cle SSH dediee et l'autorise dans dropbear (commande forcee)
  4. installe les hooks initramfs (cloudflared + hop) et regenere l'image

A lancer EN ROOT sur la machine chiffree. Affiche a la fin la configuration
a reporter dans hop (CLI ou application mobile).`,
	Run: func(cmd *cobra.Command, args []string) {
		if os.Geteuid() != 0 {
			fmt.Fprintln(os.Stderr, "Erreur: a lancer en root (sudo hop unlock setup)")
			os.Exit(1)
		}

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur config: %v\n", err)
			os.Exit(1)
		}
		if cfg.Cloudflare.EnvFile != "" {
			loadEnvFile(config.ExpandPath(cfg.Cloudflare.EnvFile))
		}

		machine := setupMachineName
		if machine == "" {
			h, _ := os.Hostname()
			machine = strings.Split(h, ".")[0]
		}
		if !isSafeName(machine) {
			fmt.Fprintf(os.Stderr, "Nom de machine invalide: %s (lettres, chiffres, - et _)\n", machine)
			os.Exit(1)
		}

		fmt.Println("→ Etape 0: verifications")
		if err := preflight(); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur: %v\n", err)
			os.Exit(1)
		}

		// Ne JAMAIS ecraser en silence une integration initramfs existante :
		// elle peut avoir ete ecrite a la main (contournements specifiques a
		// la machine) et son remplacement ne se verrait qu'au prochain boot,
		// c'est-a-dire au pire moment.
		conflicts, err := initrd.Conflicts()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur analyse initramfs: %v\n", err)
			os.Exit(1)
		}
		hmacPath := filepath.Join(unlockDir, "hmac.key")
		_, hmacExists := os.Stat(hmacPath)

		if len(conflicts) > 0 && !setupSkipInitrd {
			fmt.Println("\n⚠ Une integration initramfs DIFFERENTE est deja en place :")
			for _, c := range conflicts {
				fmt.Printf("    %s (%d octets)\n", c.Path, c.Size)
			}
			fmt.Println("\n  Ces fichiers seraient remplaces par ceux de hop. S'ils ont ete")
			fmt.Println("  ecrits a la main, leurs specificites seraient perdues — et tu ne")
			fmt.Println("  t'en apercevrais qu'au prochain demarrage bloque.")
			if !setupForce {
				fmt.Println("\n  Relance avec --force pour les remplacer (une sauvegarde sera faite),")
				fmt.Println("  ou avec --skip-initramfs pour ne pas y toucher.")
				os.Exit(1)
			}
			backupDir := filepath.Join("/root/hop-unlock-backups", time.Now().Format("20060102-150405"))
			if err := initrd.Backup(backupDir); err != nil {
				fmt.Fprintf(os.Stderr, "Erreur sauvegarde: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("\n  → Sauvegarde des fichiers existants: %s\n", backupDir)
		}
		cfPath, err := cf.EnsureInstalled()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur cloudflared: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("  cloudflared: %s\n", cfPath)

		env, err := cfaccess.LoadCFEnv(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur Cloudflare: %v\n(configure d'abord: hop config)\n", err)
			os.Exit(1)
		}
		tunnelName := "unlock-" + machine
		hostname := tunnelName + "." + env.Domain

		// --- 1. Tunnel + DNS -------------------------------------------------
		fmt.Printf("\n→ Etape 1: tunnel Cloudflare '%s'\n", tunnelName)
		info, err := cfaccess.CreateTunnel(env, tunnelName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur tunnel: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("  tunnel id: %s\n", info.ID)

		if err := os.MkdirAll(unlockDir, 0700); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur creation %s: %v\n", unlockDir, err)
			os.Exit(1)
		}
		credsPath := filepath.Join(unlockDir, "tunnel-credentials.json")
		if info.Secret != "" {
			creds, _ := json.MarshalIndent(map[string]string{
				"AccountTag":   env.AccountID,
				"TunnelSecret": info.Secret,
				"TunnelID":     info.ID,
			}, "", "  ")
			if err := os.WriteFile(credsPath, creds, 0600); err != nil {
				fmt.Fprintf(os.Stderr, "Erreur ecriture credentials: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("  credentials: %s\n", credsPath)
		} else if _, err := os.Stat(credsPath); err != nil {
			fmt.Fprintln(os.Stderr, "  ⚠ Le tunnel existait deja et ses credentials sont introuvables.")
			fmt.Fprintln(os.Stderr, "    Supprime le tunnel dans le dashboard Cloudflare puis relance.")
			os.Exit(1)
		} else {
			fmt.Println("  credentials existantes reutilisees")
		}
		_ = os.WriteFile(filepath.Join(unlockDir, "tunnel.id"), []byte(info.ID+"\n"), 0644)

		// Ingress cote API : dropbear ecoute sur 2222 dans l'initramfs.
		if err := cfaccess.ConfigureIngress(env, info.ID, hostname, "ssh://127.0.0.1:2222"); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur ingress: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("  ingress: %s -> ssh://127.0.0.1:2222\n", hostname)

		zoneID, err := cfaccess.GetZoneID(env, env.Domain)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur zone DNS: %v\n", err)
			os.Exit(1)
		}
		if err := cfaccess.CreateDNSRecord(env, zoneID, hostname, info.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Erreur DNS: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("  DNS: %s\n", hostname)

		// --- 2. Access (app + service token + policy non_identity) -----------
		fmt.Println("\n→ Etape 2: Cloudflare Access")
		res, err := cfaccess.Setup(cfg, tunnelName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erreur Access: %v\n", err)
			os.Exit(1)
		}
		if res.Reused && res.TokenSecret == "" {
			fmt.Println("  ⚠ Service token existant : son secret n'est pas recuperable via l'API.")
			fmt.Println("    Supprime-le dans le dashboard CF et relance si tu ne l'as plus.")
		}

		// --- 3. Cle SSH dediee + dropbear -----------------------------------
		fmt.Println("\n→ Etape 3: cle SSH dediee et autorisation dropbear")
		// Une cle deja autorisee pour cette machine signifie qu'un client est
		// deja configure. En generer une nouvelle le casserait sans prevenir
		// (la partie privee n'est jamais conservee ici, impossible de la
		// re-afficher) : on ne remplace que sur demande explicite.
		var privPEM string
		if hasDropbearKey(machine) && !setupRegenKey {
			fmt.Println("  cle existante conservee (--regen-key pour en generer une nouvelle)")
		} else {
			pub, priv, err := generateEd25519()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Erreur generation cle: %v\n", err)
				os.Exit(1)
			}
			if err := authorizeInDropbear(pub, machine); err != nil {
				fmt.Fprintf(os.Stderr, "Erreur dropbear: %v\n", err)
				os.Exit(1)
			}
			privPEM = priv
			fmt.Println("  cle autorisee avec command=\"/usr/bin/cryptroot-unlock\" (aucun shell possible)")
			if setupRegenKey {
				fmt.Println("  ⚠ ancienne cle revoquee : les clients existants doivent etre reconfigures")
			}
		}

		// --- 4. Config locale de notification -------------------------------
		fmt.Println("\n→ Etape 4: configuration locale")
		writeFile(filepath.Join(unlockDir, "machine"), machine, 0644)
		// La cle HMAC est partagee avec le Worker (secret UNLOCK_HMAC_SECRET_*).
		// La regenerer casserait les notifications sans rien dire : on ne la
		// remplace que si elle n'existe pas, ou sur demande explicite.
		if hmacExists != nil || setupRegenHMAC {
			hmacKey := make([]byte, 32)
			rand.Read(hmacKey)
			writeFile(hmacPath, hex.EncodeToString(hmacKey), 0600)
			if setupRegenHMAC {
				fmt.Println("  ⚠ cle HMAC regeneree : mets a jour le secret cote Worker")
				fmt.Printf("    wrangler secret put UNLOCK_HMAC_SECRET_%s\n", strings.ToUpper(machine))
			}
		} else {
			fmt.Println("  cle HMAC existante conservee (--regen-hmac pour en generer une nouvelle)")
		}
		if setupWorkerURL != "" {
			writeFile(filepath.Join(unlockDir, "worker.url"), setupWorkerURL, 0644)
		}
		if setupNtfyTopic != "" {
			ntfy := setupNtfyTopic
			if !strings.HasPrefix(ntfy, "http") {
				ntfy = "https://ntfy.sh/" + ntfy
			}
			writeFile(filepath.Join(unlockDir, "ntfy.url"), ntfy, 0644)
			fmt.Printf("  notification ntfy: %s\n", ntfy)
		}
		fmt.Printf("  %s\n", unlockDir)

		// --- 5. Initramfs ----------------------------------------------------
		if setupSkipInitrd {
			fmt.Println("\n→ Etape 5: initramfs ignore (--skip-initramfs)")
		} else {
			fmt.Println("\n→ Etape 5: integration initramfs")
			if err := initrd.Install(); err != nil {
				fmt.Fprintf(os.Stderr, "Erreur initramfs: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("  hooks installes, regeneration de l'image...")
			out, err := initrd.Rebuild()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Erreur update-initramfs: %v\n%s\n", err, out)
				os.Exit(1)
			}
			fmt.Println("  image regeneree")
		}

		// --- 6. Restitution --------------------------------------------------
		fmt.Println("\n═══════════════════════════════════════════════════════")
		fmt.Println(" Configuration a reporter sur ton client (PC ou mobile)")
		fmt.Println("═══════════════════════════════════════════════════════")
		fmt.Println("\nDans ~/.hop/config.yml (ou /tmp/hop-<uid>/config.yml) :")
		fmt.Println("\nunlock:")
		fmt.Printf("    - name: %s\n", machine)
		fmt.Printf("      hostname: %s\n", hostname)
		fmt.Printf("      token_id: %s\n", res.TokenID)
		if res.TokenSecret != "" {
			fmt.Printf("      token_secret: %s\n", res.TokenSecret)
		} else {
			fmt.Println("      token_secret: <secret existant, non recuperable>")
		}
		fmt.Println("      key_file: ~/.ssh/hop_unlock_" + machine)
		if hk := dropbearHostKey(); hk != "" {
			fmt.Printf("      host_key: %s\n", hk)
		} else {
			fmt.Println("      # host_key: <cle hote dropbear illisible — verifie dropbearkey>")
		}
		if privPEM != "" {
			fmt.Printf("\nCle privee a enregistrer dans ~/.ssh/hop_unlock_%s (chmod 600) :\n\n", machine)
			fmt.Println(privPEM)
		} else {
			fmt.Printf("\nCle SSH inchangee : garde celle deja presente dans ~/.ssh/hop_unlock_%s\n", machine)
		}
		fmt.Println("Teste ensuite, machine eteinte puis rallumee :  hop unlock " + machine)
		if setupNtfyTopic == "" {
			fmt.Println("\nAstuce: --ntfy <topic> pour etre prevenu quand la machine attend sa passphrase.")
		}
	},
}

func preflight() error {
	if _, err := os.Stat("/etc/dropbear/initramfs"); err != nil {
		return fmt.Errorf("dropbear-initramfs absent (apt install dropbear-initramfs)")
	}
	if _, err := exec.LookPath("update-initramfs"); err != nil {
		return fmt.Errorf("update-initramfs introuvable (systeme non Debian/Ubuntu ?)")
	}
	// Attention : /usr/bin/cryptroot-unlock n'existe QUE dans l'image
	// initramfs, pas sur le systeme demarre. Le marqueur cote systeme est le
	// binaire source fourni par cryptsetup-initramfs.
	if _, err := os.Stat("/usr/share/cryptsetup/initramfs/bin/cryptroot-unlock"); err != nil {
		return fmt.Errorf("cryptsetup-initramfs non installe (apt install cryptsetup-initramfs)")
	}
	if _, err := os.Stat("/etc/crypttab"); err != nil {
		return fmt.Errorf("/etc/crypttab absent : cette machine n'a pas de volume chiffre au boot")
	}
	return nil
}

func isSafeName(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func generateEd25519() (authorizedKey string, privatePEM string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return "", "", err
	}
	pemBlock, err := ssh.MarshalPrivateKey(priv, "hop-unlock")
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))),
		strings.TrimSpace(string(pem.EncodeToMemory(pemBlock))), nil
}

// dropbearHostKey retourne la cle hote publique de dropbear (format
// "ssh-ed25519 AAAA..."), a epingler cote client. Contrairement a une idee
// repandue, dropbear NE regenere PAS cette cle a chaque boot : elle est
// persistee dans /etc/dropbear/initramfs/ et embarquee dans l'image.
func dropbearHostKey() string {
	for _, name := range []string{"dropbear_ed25519_host_key", "dropbear_ecdsa_host_key"} {
		out, err := exec.Command("dropbearkey", "-y", "-f",
			"/etc/dropbear/initramfs/"+name).Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "ssh-") {
				// On ne garde que "<type> <base64>" : le commentaire final
				// (root@machine) n'a pas sa place dans un known_hosts epingle.
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					return parts[0] + " " + parts[1]
				}
			}
		}
	}
	return ""
}

// hasDropbearKey indique si une cle hop est deja autorisee pour cette machine.
func hasDropbearKey(machine string) bool {
	data, err := os.ReadFile("/etc/dropbear/initramfs/authorized_keys")
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "hop-unlock-"+machine)
}

func authorizeInDropbear(pubKey, machine string) error {
	path := "/etc/dropbear/initramfs/authorized_keys"
	existing, _ := os.ReadFile(path)

	// Idempotent : on retire nos precedentes cles pour cette machine avant
	// d'ajouter la nouvelle. Sans ca, chaque relance du setup laissait une
	// cle orpheline autorisee a vie (dont plus personne n'a la partie privee).
	tag := "hop-unlock-" + machine
	var kept []string
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == "" || strings.HasSuffix(strings.TrimSpace(line), tag) {
			continue
		}
		kept = append(kept, line)
	}

	// Commande forcee : cette cle ne peut RIEN faire d'autre que demander la
	// passphrase — pas de shell, pas de port forwarding.
	kept = append(kept, fmt.Sprintf(
		"command=\"/usr/bin/cryptroot-unlock\",no-port-forwarding,no-agent-forwarding,no-X11-forwarding %s %s",
		pubKey, tag))

	return os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0600)
}

func writeFile(path, content string, mode os.FileMode) {
	os.WriteFile(path, []byte(content+"\n"), mode)
}

func init() {
	unlockSetupCmd.Flags().StringVar(&setupMachineName, "machine", "", "nom de la machine (defaut: hostname)")
	unlockSetupCmd.Flags().StringVar(&setupNtfyTopic, "ntfy", "", "topic ntfy pour etre prevenu (ou URL complete)")
	unlockSetupCmd.Flags().StringVar(&setupWorkerURL, "worker", "", "URL du worker hop pour l'etat d'attente")
	unlockSetupCmd.Flags().BoolVar(&setupSkipInitrd, "skip-initramfs", false, "ne pas toucher a l'initramfs")
	unlockSetupCmd.Flags().BoolVar(&setupForce, "force", false, "remplacer une integration initramfs existante (avec sauvegarde)")
	unlockSetupCmd.Flags().BoolVar(&setupRegenHMAC, "regen-hmac", false, "regenerer la cle HMAC (invalide le secret cote Worker)")
	unlockSetupCmd.Flags().BoolVar(&setupRegenKey, "regen-key", false, "regenerer la cle SSH (invalide les clients deja configures)")
	unlockCmd.AddCommand(unlockSetupCmd)
}
