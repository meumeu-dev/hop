# hop

> **Beta** — en cours de dev, ca peut bouger. Feedback bienvenu via les [issues](https://github.com/meumeu-dev/hop/issues).

Lanceur de commandes, SSH et config perso. Un seul binaire pour gerer toutes tes machines.

## Installation

```bash
curl -sSL https://raw.githubusercontent.com/meumeu-dev/hop/master/install.sh | bash
```

### Mise a jour

```bash
hop update
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
| `hop <service> [machine]` | Lance un service (local ou distant) |
| `hop pair` | Pairing securise (auto/lan/relay/gist) |
| `hop ssh <machine>` | Connexion SSH intelligente |
| `hop ping [machine]` | Verifie l'etat des machines |
| `hop list` | Liste services et machines |
| `hop add machine/service` | Ajoute une machine ou service |
| `hop remove <nom>` | Supprime |
| `hop rename <ancien> <nouveau>` | Renomme |
| `hop alias add <alias> <cible>` | Raccourci |
| `hop tunnel quick` | Tunnel temporaire (multi-provider) |
| `hop tunnel setup` | Tunnel Cloudflare permanent |
| `hop dashboard` | Interface web (local/reseau/tunnel) |
| `hop export` | Backup config chiffre |
| `hop import <source>` | Restaure config |
| `hop server` | Relay de pairing self-hosted |
| `hop worker deploy/url` | Worker custom |
| `hop update` | Met a jour (changelog + checksum) |
| `hop version` | Affiche la version |
| `hop reset` | Remet la config a zero |
| `hop uninstall` | Supprime hop |
| `hop completion` | Autocompletion bash/zsh/fish |

## Pairing

4 modes, selection interactive :

```bash
hop pair              # menu: auto/lan/relay/gist
hop pair -m lan       # LAN uniquement
hop pair -m relay     # relay worker uniquement
hop pair -m gist      # GitHub Gist (necessite gh CLI)
```

- **Auto** : LAN + relay en parallele, premier qui repond gagne
- **LAN** : broadcast UDP, zero internet
- **Relay** : worker Cloudflare chiffre E2E
- **Gist** : Gist GitHub prive comme relay

Chiffrement AES-GCM + Argon2id, code 8 chars alphanumerique, verification checksum SHA256.

## Tunnels

5 providers, selection interactive :

```bash
hop tunnel quick                # menu provider
hop tunnel quick -p localhost.run  # skip menu
```

| Provider | Install | Compte requis |
|----------|---------|---------------|
| trycloudflare | auto cloudflared | non |
| localhost.run | zero (SSH) | non |
| bore.pub | auto bore | non |
| Cloudflare | auto cloudflared | oui (gratuit) |
| Worker perso | - | oui |

Si un provider echoue, repropose le menu.

## Dashboard

```bash
hop dashboard              # menu: localhost / reseau / tunnel
hop dashboard --bind 0.0.0.0  # reseau (mot de passe requis)
```

## Export / Import

```bash
hop export                 # fichier local chiffre
hop export --cloud         # upload sur worker (lien 2min)
hop import backup.enc      # depuis fichier
hop import cloud:<id>      # depuis worker
```

## Self-hosted

```bash
hop server --port 8899     # relay standalone
hop worker deploy          # deploie ton worker CF
hop worker url <url>       # configure relay custom
```

## Config

`~/.hop/config.yml` (config) + `~/.hop/secrets.yml` (secrets, gitignore).

## License

MIT
