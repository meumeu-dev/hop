# hop

> **Beta** — en cours de dev. Feedback bienvenu via les [issues](https://github.com/meumeu-dev/hop/issues).

Un seul binaire pour gerer toutes tes machines. SSH, pairing chiffre, tunnels Cloudflare.

## Installation

```bash
curl -sSL https://raw.githubusercontent.com/meumeu-dev/hop/master/install.sh | bash
```

## Demarrage rapide

```bash
hop init

# Pairer deux machines
hop pair          # machine A
hop pair <token>  # machine B

# Se connecter
hop ssh rpi
```

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
| `hop tunnel setup` | Tunnel Cloudflare permanent |
| `hop dashboard` | Interface web |
| `hop export [--cloud]` | Backup config chiffre |
| `hop import <source>` | Restaure config |
| `hop worker url [url]` | Configure worker custom |
| `hop update [-y]` | Mise a jour + checksum SHA256 |
| `hop reset/uninstall` | Cleanup |
| `hop completion` | Autocompletion |

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
# Puis configure l'URL
hop worker url https://hop-pair.ton-domaine.workers.dev
```

## Tunnels

```bash
hop tunnel setup    # Cloudflare Tunnel permanent + systemd
```

Necessite un compte Cloudflare (gratuit) + un domaine.

## Dashboard

```bash
hop dashboard                     # localhost
hop dashboard --bind 0.0.0.0      # reseau (mot de passe requis)
```

## Config

`~/.hop/config.yml` + `~/.hop/secrets.yml`

## License

MIT
