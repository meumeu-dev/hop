# hop

> **Beta** — en cours de dev. Feedback bienvenu via les [issues](https://github.com/meumeu-dev/hop/issues).

Un seul binaire pour gerer toutes tes machines. SSH, pairing chiffre E2E, tunnels Cloudflare.

## Installation

```bash
curl -sSL https://raw.githubusercontent.com/meumeu-dev/hop/master/install.sh | bash
```

## Demarrage rapide

```bash
hop init                  # configure hop (mode sandbox par defaut)

hop pair                  # machine A — affiche un token
hop pair <token>          # machine B — se connecte

hop ssh rpi               # SSH (auto LAN ou tunnel)

hop install               # rend permanent (survit au reboot)
hop exit                  # supprime toute trace (mode sandbox)
```

## Sandbox vs Installe

Par defaut hop est en **mode sandbox** : la config est dans `/tmp/` et disparait au reboot.

| | Sandbox (defaut) | Installe (`hop install`) |
|---|---|---|
| Config | `/tmp/hop-<uid>/` | `~/.hop/` |
| Reboot | Config perdue | Config persistante |
| Tunnel | Foreground | Service systemd |
| Nettoyage | `hop exit` (zero trace) | `hop uninstall` |

## Commandes

| Commande | Description |
|----------|-------------|
| `hop <service> [machine]` | Lance un service |
| `hop pair` | Pairing securise (auto/lan/relay) |
| `hop ssh <machine>` | Connexion SSH (LAN/tunnel) |
| `hop ping [machine]` | Status des machines |
| `hop list` | Liste tout |
| `hop add machine/service` | Ajoute |
| `hop remove/rename/alias` | Gestion |
| `hop config cf` | Configure Cloudflare (domaine + token) |
| `hop config show` | Affiche la config |
| `hop tunnel setup` | Tunnel Cloudflare permanent |
| `hop dashboard` | Interface web |
| `hop export [--cloud]` | Backup config chiffre |
| `hop import <source>` | Restaure config |
| `hop worker url [url]` | Configure worker custom |
| `hop update [-y]` | Mise a jour + checksum SHA256 |
| `hop install` | Rend permanent (survit au reboot) |
| `hop exit` | Supprime toute trace (sandbox) |
| `hop reset/uninstall` | Cleanup |
| `hop completion` | Autocompletion |

## Cloudflare

```bash
hop config cf             # configure domaine + token API en une fois
hop tunnel setup          # cree un tunnel permanent + systemd
```

Necessite un compte Cloudflare (gratuit) + un domaine. `hop init` propose de configurer CF au demarrage.

## Pairing

3 modes :
- **Auto** (`hop pair`) : LAN + relay en parallele
- **LAN** (`hop pair -m lan`) : broadcast UDP, zero internet
- **Relay** (`hop pair -m relay`) : worker Cloudflare

Chiffrement AES-GCM + Argon2id. Le relay ne voit jamais les donnees en clair (E2E).

### Worker custom

Par defaut, hop utilise le relay `hop-pair.meumeudev.workers.dev`. Pour utiliser ton propre relay :

```bash
# Deploie le worker sur ton compte CF (code source dans worker/)
hop worker url https://hop-pair.ton-domaine.workers.dev
```

## Dashboard

```bash
hop dashboard                     # localhost
hop dashboard --bind 0.0.0.0      # reseau (mot de passe 8+ chars)
```

## Config

`~/.hop/config.yml` — config principale
`~/.hop/cloudflare.env` — credentials CF (gitignore)

## License

MIT
