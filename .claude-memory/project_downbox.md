---
name: project_downbox
description: DownBox - download station Go, aria2, tunnels, DoH, blocklist, Docker, pentest 6 rounds
type: project
---

DownBox — Lightweight self-hosted download station with web UI.
Repo: /home/freelux/projets/downbox/ — GitHub: meumeu-dev/downbox
Landing page: GitHub Pages (docs/ → gh-pages branch), meumeu.dev/downbox redirige dessus.

**Stack**: Go (0 deps externes) + Alpine.js, aria2 subprocess, single binary ~7.5MB
**Archi**: amd64, i386, arm64, armv7. Docker multi-stage Alpine ~44MB.
**Version actuelle**: v0.7.1

**Features**:
- aria2 engine (HTTP, FTP, BitTorrent, magnet, 16 conn/dl)
- File browser + upload avec progress, preview, rename, delete
- Share links (local/public via tunnel, token 128-bit)
- Tunnels: Cloudflare Tunnel (token) ou Bore (custom server+secret)
- DNS-over-HTTPS intégré (Cloudflare, Google, Quad9, Mullvad, NextDNS, custom)
- IP blocklist intégré (SOCKS5 proxy filtrant, listes pré-configurées)
- VPN interface binding (--interface pour aria2)
- Proxy SOCKS5/HTTP pour downloads
- Docker support (DOWNBOX_BIND=0.0.0.0, healthcheck, non-root)
- Password auth obligatoire (salted SHA256 hash, HMAC session cookies persistants)

**Sécurité** (6 rounds red team/blue team, 50+ vulns corrigées):
- SSRF: DNS pinning, redirect resolution server-side, IP formats, blocklist URL validation
- Auth: HMAC signed cookies (session-secret en config), rate limiting login
- Path traversal: safePath, EvalSymlinks, O_NOFOLLOW, /proc/self/fd
- XSS: CSP, Content-Type enforcement, Alpine.js x-text
- Config: 0600, sanitizeConfigValue, password hash (jamais plaintext)
- Data races: sync.RWMutex sur Config, field-by-field copy
- aria2 options whitelist (pas de header/dir/out injection)
- Headers sécu: X-Frame-Options, CSP, nosniff, Referrer-Policy

**Infra RPi (192.168.0.2)**:
- Installé en v0.7.0, port 8090, systemd service
- Password: 0265fde6c9bea7938e389eea
- VPNPlex supprimé du RPi (était sur port 8080)
- Config: ~/.config/downbox/downbox.conf

**Déploiement**:
- GitHub Actions: tag v* → build 4 archi → GitHub Release
- install.sh: curl | bash, PORT/ARIA2_PORT configurable, mktemp
- downbox update: détecte systemd, sudo systemctl stop/start
- meumeu.dev: CF Pages (Direct Upload), _redirects vers GH Pages + install.sh
- Docker: Dockerfile + docker-compose.yml dans le repo

**Why:** Projet perso download station, alternative légère aux seedbox compliquées. Créneau = tout-en-un single binary sans config.
**How to apply:** Toujours vérifier go build + go vet. Pentest après chaque feature. Versionner avec tag. Nettoyer les anciennes releases.
