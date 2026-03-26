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
# Initialiser hop
hop init

# Pairer deux machines (meme reseau)
hop pair          # machine A — affiche un code
hop pair 123456   # machine B — entre le code

# Se connecter
hop ssh rpi
```

## Commandes

| Commande | Description |
|----------|-------------|
| `hop <service> [machine]` | Lance un service (local ou distant) |
| `hop list` | Liste services et machines |
| `hop add machine <nom> <ip>` | Ajoute une machine |
| `hop add service <nom> --cmd <cmd>` | Ajoute un service |
| `hop remove <nom>` | Supprime une machine ou service |
| `hop rename <ancien> <nouveau>` | Renomme une machine ou service |
| `hop alias add <alias> <cible>` | Cree un raccourci |
| `hop ping [machine]` | Verifie l'etat des machines |
| `hop ssh <machine>` | Connexion SSH (auto LAN/tunnel) |
| `hop pair` | Pairing securise entre machines |
| `hop tunnel quick` | Tunnel temporaire (zero config CF) |
| `hop tunnel setup` | Configure un Cloudflare Tunnel permanent |
| `hop server` | Lance un relay de pairing self-hosted |
| `hop worker deploy` | Deploie ton propre worker CF |
| `hop worker url [url]` | Configure l'URL du worker |
| `hop dashboard` | Lance le dashboard web |
| `hop api` | Active l'API REST |
| `hop remote add <nom> <url>` | Ajoute un hop distant |
| `hop version` | Affiche la version |
| `hop update` | Met a jour hop |
| `hop reset` | Remet la config a zero |
| `hop uninstall` | Supprime hop completement |
| `hop completion [bash\|zsh\|fish]` | Autocompletion shell |

## Pairing

Le pairing connecte deux machines de maniere securisee. 3 modes disponibles :

### LAN (par defaut si meme reseau)
```bash
# Machine A:
hop pair
# → Recherche sur le reseau local...
# → Code: 123456

# Machine B:
hop pair 123456
```
Aucun internet requis. Broadcast UDP + echange direct.

### Worker relay (par defaut si pas en LAN)
```bash
# Machine A:
hop pair
# → Bascule sur le relay...
# → Token: abc123.456789.xyz...

# Machine B:
hop pair abc123.456789.xyz...
```
Utilise un relay Cloudflare Worker chiffre E2E. Le relay ne voit jamais les donnees en clair.

### GitHub Gist (sans worker)
```bash
# Machine A:
hop pair --gist
# → Token: gist:abc123def.456789

# Machine B:
hop pair gist:abc123def.456789
```
Utilise un Gist GitHub prive comme relay. Necessite `gh` CLI.

### Securite
- Chiffrement AES-GCM avec derivation Argon2id (64MB, 3 iterations)
- Echange de cles SSH ed25519
- Verification d'empreinte SSH des deux cotes
- Confirmation manuelle requise

## Connexion intelligente

Hop detecte automatiquement le meilleur chemin :
1. **LAN** — ping direct sur le port SSH
2. **Tunnel configure** — Cloudflare Tunnel permanent
3. **Tunnel dynamique** — resolution via le worker (trycloudflare)

## Tunnels

### Tunnel temporaire (zero config)
```bash
hop tunnel quick
# → Lance un tunnel trycloudflare, enregistre l'URL sur le worker
# → Les autres machines resolvent automatiquement
```
Aucun compte Cloudflare requis.

### Tunnel permanent
Necessite un compte Cloudflare (gratuit) + un domaine. Voir la section [Cloudflare setup](#prerequis-pour-les-tunnels-permanents).

## Self-hosted relay

Pour ceux qui ne veulent pas utiliser le worker par defaut :

```bash
# Option 1: deployer son propre worker CF (gratuit)
hop worker deploy

# Option 2: relay standalone (sur un VPS, raspi, etc.)
hop server --port 8899

# Configurer l'URL
hop worker url https://mon-relay.example.com:8899
```

## Services & Tmux

```bash
# Ajouter un service
hop add service code --cmd "code ." --desc "VS Code"
hop code pc1

# Lancer dans tmux
hop code pc1 --tmux --session dev
```

## Dashboard & API

```bash
hop dashboard           # Interface web locale
hop api --port 9090     # API REST pour federation
```

## Alias

```bash
hop alias add rpi raspberrypi
hop ssh rpi     # → se connecte a raspberrypi
```

## Prerequis pour les tunnels permanents

Les tunnels Cloudflare permettent d'acceder a tes machines depuis n'importe ou, sans ouvrir de port. C'est optionnel.

1. Compte Cloudflare gratuit sur [dash.cloudflare.com](https://dash.cloudflare.com)
2. Domaine ajoute dans Cloudflare (plan Free)
3. Token API : [dash.cloudflare.com/profile/api-tokens](https://dash.cloudflare.com/profile/api-tokens) → template "Edit zone DNS"
4. Configurer dans hop : `hop init` ou `hop dashboard`

Hop gere automatiquement la creation des tunnels, le DNS et cloudflared.

## Config

La config est dans `~/.hop/config.yml`. Les secrets (cles API) sont dans `~/.hop/secrets.yml` (gitignore).

## License

MIT
