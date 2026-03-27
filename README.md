# hop

> **Beta** — Feedback bienvenu via les [issues](https://github.com/meumeu-dev/hop/issues).

Un seul binaire pour gerer toutes tes machines. **Sandboxe par defaut** — zero trace au reboot. SSH, pairing chiffre E2E, tunnels Cloudflare.

## Installation

**Linux & macOS** (amd64, arm64 — arm32 Linux only):

```bash
curl -sL meumeu.dev/hop/install | bash
```

**Windows** (PowerShell):

```powershell
iwr -Uri "https://github.com/meumeu-dev/hop/releases/latest/download/hop-windows-amd64.exe" -OutFile hop.exe
.\hop.exe config
```

Or download `hop-windows-amd64.exe` manually from the [releases page](https://github.com/meumeu-dev/hop/releases).

## Plateformes supportees

| OS      | amd64 | arm64 | arm32 |
|---------|-------|-------|-------|
| Linux   | yes   | yes   | yes   |
| macOS   | yes   | yes   | —     |
| Windows | yes   | —     | —     |

## Demarrage rapide

```bash
hop config                  # configure hop (sandbox par defaut)

hop pair                  # machine A — affiche un token
hop pair <token>          # machine B — se connecte

hop ssh rpi               # SSH (auto LAN ou tunnel)

hop install               # rend permanent (survit au reboot)
hop exit                  # supprime toute trace (furtif)
```

## Sandbox vs Installe

Par defaut hop est en **mode sandbox** : la config est dans `/tmp/` et disparait au reboot.

| | Sandbox (defaut) | Installe (`hop install`) |
|---|---|---|
| Config | `/tmp/hop-<uid>/` | `~/.hop/` |
| Reboot | Config perdue | Config persistante |
| Tunnel | Foreground | Service systemd |
| Nettoyage | `hop exit` (zero trace) | `hop uninstall` (nucleaire) |

## Commandes

### Connexion
| Commande | Description |
|----------|-------------|
| `hop <service> [machine]` | Lance un service (local ou distant) |
| `hop ssh <machine>` | Connexion SSH (auto LAN/tunnel) |
| `hop ping [machine]` | Verifie l'etat des machines |
| `hop list` | Liste machines, services, aliases |
| `hop send <machine> <fichier-ou-url>` | Envoie fichier ou telecharge URL sur la machine |
| `hop receive <machine> <chemin>` | Recoit un fichier depuis la machine |

### Pairing
| Commande | Description |
|----------|-------------|
| `hop pair` | Pairing securise (menu: auto/lan/relay) |
| `hop pair -m lan` | LAN uniquement (broadcast UDP) |
| `hop pair -m relay` | Relay worker uniquement |

### Gestion
| Commande | Description |
|----------|-------------|
| `hop add machine <nom> <ip> --user <user>` | Ajoute une machine |
| `hop add service <nom> --cmd <cmd>` | Ajoute un service |
| `hop remove <nom>` | Supprime machine ou service |
| `hop rename <ancien> <nouveau>` | Renomme |
| `hop alias add <alias> <cible>` | Cree un raccourci |
| `hop alias list` | Liste les alias |

### Cloudflare
| Commande | Description |
|----------|-------------|
| `hop config` | Configure Cloudflare + worker (interactif) |
| `hop config --env <fichier-ou-url>` | Importe un .env CF |
| `hop config --show` | Affiche la config actuelle |
| `hop tunnel setup` | Cree un tunnel Cloudflare permanent |
| `hop tunnel quick` | Tunnel rapide via Pinggy (zero install, zero compte) |
| `hop tunnel status` | Status des tunnels |

### Sauvegarde
| Commande | Description |
|----------|-------------|
| `hop export` | Backup config chiffre (fichier local) |
| `hop export --cloud` | Backup chiffre sur le worker (lien 2min) |
| `hop import <fichier-ou-token>` | Restaure config |

### Systeme
| Commande | Description |
|----------|-------------|
| `hop install` | Rend permanent (~/.hop/, survit au reboot) |
| `hop exit` | Furtif: supprime config + binaire, zero trace |
| `hop uninstall` | Nucleaire: supprime TOUT (config + services + cloudflared + binaire) |
| `hop reset [-y]` | Remet la config a zero |
| `hop update [-y]` | Mise a jour + changelog + checksum SHA256 |
| `hop version [--check]` | Affiche la version |
| `hop dashboard` | Interface web (local / reseau / tunnel) |
| `hop completion [bash\|zsh\|fish]` | Autocompletion shell |
| `hop ai <question>` | Assistant AI (opt-in, Ollama local ou Workers AI) |
| `hop ai --enable/--disable` | Active/desactive l'AI |

## Pairing

3 modes, chiffrement AES-GCM + Argon2id :
- **Auto** : LAN + relay en parallele
- **LAN** : broadcast UDP, zero internet
- **Relay** : worker Cloudflare E2E

Le relay ne voit jamais les donnees en clair.

### Worker custom

Le worker custom se configure dans `hop config` (question interactive).

## Tunnels

```bash
hop config             # configure CF
hop tunnel setup          # cree le tunnel + DNS
```

## Dashboard

```bash
hop dashboard                     # localhost
hop dashboard --bind 0.0.0.0      # reseau (mot de passe 8+ chars)
```

## Config

`~/.hop/config.yml` (installe) ou `/tmp/hop-<uid>/config.yml` (sandbox)

## License

MIT
