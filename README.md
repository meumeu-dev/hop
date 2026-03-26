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

### Build depuis les sources

```bash
go build -ldflags "-X main.Version=v1.1.0" -o hop .
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
| `hop update` | Met a jour hop |
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

## Prerequis pour les tunnels

Les tunnels Cloudflare permettent d'acceder a tes machines depuis n'importe ou, sans ouvrir de port. C'est optionnel — hop marche en LAN sans Cloudflare.

### 1. Creer un compte Cloudflare

Inscris-toi sur [dash.cloudflare.com](https://dash.cloudflare.com). C'est gratuit.

### 2. Ajouter un domaine

- Va dans **Websites** > **Add a site**
- Entre ton domaine (ex: `mondomaine.dev`)
- Choisis le plan **Free**
- Cloudflare te donne 2 nameservers (ex: `anna.ns.cloudflare.com`)
- Va chez ton registrar (OVH, Gandi, Namecheap...) et remplace les NS par ceux de Cloudflare
- Attends la propagation (quelques minutes a 24h)

### 3. Recuperer le token API

- Va dans [dash.cloudflare.com/profile/api-tokens](https://dash.cloudflare.com/profile/api-tokens)
- Clique **Create Token**
- Utilise le template **Edit zone DNS** :
  - Permissions : `Zone > DNS > Edit`
  - Zone Resources : `Include > Specific zone > ton domaine`
- Clique **Continue to summary** > **Create Token**
- Copie le token (il ne sera plus affiche)

### 4. Recuperer le Account ID

- Va sur ta zone (clique sur ton domaine dans le dashboard)
- L'**Account ID** est dans la colonne de droite, section **API**
- Copie-le

### 5. Configurer hop

```bash
hop init
# -> Entre ton domaine quand demande

# Ou via le dashboard
hop dashboard
# -> Onglet Cloudflare, entre domaine + email + token API
```

Cree un fichier `~/.hop/cloudflare.env` :

```
CF_USER=ton@email.com
CF_DOMAIN=mondomaine.dev
CF_API_KEY=ton-token-ici
CF_ACCOUNT_ID=ton-account-id
```

### 6. Creer un tunnel

```bash
hop tunnel setup mon-pc
# -> Suit les instructions (auth Cloudflare + DNS automatique)
```

Chaque machine aura un hostname type `mon-pc.mondomaine.dev`. Hop bascule automatiquement entre LAN et tunnel selon la disponibilite.

## Config

La config est dans `~/.hop/config.yml`. Les secrets (cles API) sont dans `~/.hop/secrets.yml` (gitignore).

## License

MIT
