# hop

> **Beta** — en cours de dev. Feedback bienvenu via les [issues](https://github.com/meumeu-dev/hop/issues).

Un seul binaire pour gerer toutes tes machines. SSH, pairing, tunnels Cloudflare.

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
| `hop pair` | Pairing securise (auto/lan/relay/gist) |
| `hop ssh <machine>` | Connexion SSH (auto LAN/tunnel) |
| `hop ping [machine]` | Status des machines |
| `hop list` | Liste tout |
| `hop add machine/service` | Ajoute |
| `hop remove <nom>` | Supprime |
| `hop rename <ancien> <nouveau>` | Renomme |
| `hop alias add <alias> <cible>` | Raccourci |
| `hop tunnel setup` | Tunnel Cloudflare permanent |
| `hop tunnel status` | Status des tunnels |
| `hop dashboard` | Interface web |
| `hop export [--cloud]` | Backup config chiffre |
| `hop import <source>` | Restaure config |
| `hop server` | Relay de pairing self-hosted |
| `hop update [-y]` | Mise a jour (changelog + checksum SHA256) |
| `hop reset [-y]` | Reset config |
| `hop uninstall` | Desinstalle hop |
| `hop version` | Version |
| `hop completion` | Autocompletion bash/zsh/fish |

## Pairing

4 modes :
- **Auto** (`hop pair`) : LAN + relay en parallele
- **LAN** (`hop pair -m lan`) : broadcast UDP, zero internet
- **Relay** (`hop pair -m relay`) : worker Cloudflare chiffre E2E
- **Gist** (`hop pair -m gist`) : GitHub Gist prive

Chiffrement AES-GCM + Argon2id, code 8 chars alphanumerique.

## Tunnels

```bash
hop tunnel setup    # Cloudflare Tunnel permanent + systemd
```

Necessite un compte Cloudflare (gratuit) + un domaine.

## Dashboard

```bash
hop dashboard                     # localhost
hop dashboard --bind 0.0.0.0      # reseau (mot de passe)
```

## Config

`~/.hop/config.yml` + `~/.hop/secrets.yml`

## License

MIT
