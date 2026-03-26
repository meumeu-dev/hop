# hop

Lanceur de commandes, SSH et config perso. Un seul binaire pour gerer toutes tes machines.

## Installation

```bash
# One-liner (necessite gh CLI authentifie)
bash <(gh api repos/meumeu-dev/hop/contents/install.sh --jq '.content' | base64 -d)

# Ou avec token explicite
GITHUB_TOKEN=ghp_xxx bash install.sh

# Ou build depuis les sources
go build -ldflags "-X main.Version=v1.0.0" -o hop .
```

### Mise a jour

```bash
hop update
```

## Demarrage rapide

```bash
# Initialiser hop
hop init

# Ajouter une machine
hop add machine pc1 192.168.0.10 --user freelux

# Se connecter en SSH
hop ssh pc1

# Ajouter un service
hop add service code --cmd "code ." --desc "VS Code"

# Lancer un service sur une machine
hop code pc1
```

## Commandes

| Commande | Description |
|----------|-------------|
| `hop <service> [machine]` | Lance un service (local ou distant) |
| `hop list` | Liste services et machines |
| `hop add machine <nom> <ip>` | Ajoute une machine |
| `hop add service <nom> --cmd <cmd>` | Ajoute un service |
| `hop remove <nom>` | Supprime une machine ou service |
| `hop ping [machine]` | Verifie l'etat des machines |
| `hop ssh <machine>` | Connexion SSH (auto LAN/tunnel) |
| `hop pair` | Pairing securise entre machines |
| `hop tunnel setup` | Configure un Cloudflare Tunnel |
| `hop sync` | Synchronise la config via git |
| `hop dashboard` | Lance le dashboard web |
| `hop api` | Active l'API REST |
| `hop remote add <nom> <url>` | Ajoute un hop distant |
| `hop version` | Affiche la version |
| `hop completion [bash\|zsh\|fish]` | Autocompletion shell |

## Fonctionnalites

### Connexion intelligente
Hop detecte automatiquement si une machine est joignable en LAN. Si non, il passe par le Cloudflare Tunnel configure.

### Pairing zero-config
```bash
# Sur la machine serveur:
hop pair
# -> Affiche un code a 6 chiffres

# Sur la nouvelle machine:
hop pair <code>
# -> Echange de cles SSH + config tunnel automatique
```

Le pairing utilise un chiffrement AES-GCM avec derivation Argon2id. Le relay Cloudflare Worker ne voit jamais les donnees en clair.

### Tmux
```bash
# Lancer dans tmux
hop code pc1 --tmux --session dev

# Ou configurer par defaut
hop add service code --cmd "claude" --tmux
```

### Dashboard & API
```bash
# Dashboard web local
hop dashboard

# API pour federation multi-machines
hop api --port 9090
hop remote add bureau https://bureau.example.com:9090 --key <cle>
```

### Autocompletion
```bash
# Bash
source <(hop completion bash)

# Zsh
source <(hop completion zsh)
```

## Config

La config est dans `~/.hop/config.yml`. Les secrets (cles API) sont dans `~/.hop/secrets.yml` (gitignore).

## Build

```bash
go build -o hop .

# Avec version
go build -ldflags "-X main.Version=v1.1.0" -o hop .

# Tests
go test ./...
```

## License

MIT
