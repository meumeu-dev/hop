---
name: VPNPlex Bastion
description: VPNPlex multiplexeur VPN sur bastion Debian LUKS, archi parano max, Cloudflare Tunnel, AdGuard Home, Claude Code
type: project
---

## VPNPlex — Bastion (192.168.0.20)

**Machine:** PC portable i5-2410M, 8Go RAM, 449Go disque, Debian 13 Trixie, LUKS+LVM, hostname "bastion"
**MAC:** b8:70:f4:94:d3:e8 (enp2s0f0), bail DHCP fixe sur Livebox à 192.168.0.20
**User:** freelux / tgboulet, sudo NOPASSWD
**SSH:** clé ed25519 copiée, port 22
**SSH distant:** `ssh bastion` (via Cloudflare Tunnel ssh.appstorefr.net, config dans ~/.ssh/config du PC principal)
**Dropbear:** port 2222, unlock LUKS distant via ~/unlock-bastion.sh (expect)
**Passphrase LUKS:** salefilsdepute
**Clavier:** FR (azerty), configuré dans initramfs
**Interfaces réseau:** enp2s0f0 (ethernet), wlp3s0b1 (WiFi, non utilisé), USB-Ethernet (pas encore branché)

## Architecture réseau parano max

```
Client → VPS:51820 → tunnel WG:51830 → PureVPN CH (188.240.60.x) → Bastion → sortie VPN au choix
```

- IP maison invisible partout (VPS voit IP PureVPN CH, pas l'IP maison)
- Kill switch : DROP tout trafic VPN/LAN vers enp2s0f0
- Anti DNS leak : toutes requêtes DNS redirigées vers AdGuard Home local
- Les sorties possibles : VPS Direct, PureVPN CH, PureVPN NL (ou toute autre sortie ajoutée)

## Services systemd persistants

- `openvpn@purevpn-ch` — tunnel système CH (tun0), cache IP maison au VPS, route-nopull + route VPS
- `wg-quick@wg-relay` — tunnel WG vers VPS (10.88.88.2 ↔ 10.88.88.1), dépend de purevpn-ch
- `openvpn@purevpn-nl` — sortie VPN NL (tun1), route-nopull
- `wg-quick@wg0` — serveur WG clients (10.77.77.0/24, port 51820), NAT vers tun actif
- `vpnplex-web` — interface admin Flask sur port 8080
- `vpnplex-killswitch` — kill switch iptables + anti DNS leak
- `AdGuardHome` — DNS filtré DoH/QUIC, port 53 + admin port 3000
- `cloudflared` — Cloudflare Tunnel vers vpn.appstorefr.net et ssh.appstorefr.net

## Ordre de démarrage

openvpn@purevpn-ch → wg-quick@wg-relay (sleep 5) → openvpn@purevpn-nl → wg-quick@wg0 (sleep 5)

## Clés WireGuard

### Serveur VPNPlex (wg0)
- Priv: SC4i2w+U6Nxhy221Xm0RJOvXn090qrk0WvGatiUa8HM=
- Pub: N/1MyDNzGrkvRQ33zOR5FZcAF9ReaZ92qRbl9Xz/MQI=
- Subnet: 10.77.77.0/24, port 51820

### Tunnel relay bastion
- Priv: UKsfGszsTzfCb0+2RR5uvDm6CalQXj1StxCvYIaHJmg=
- Pub: j6dc3yTBFFk4c+ZJydr5QLdGVK92ZXZPMebfdvCmPBM=
- IP: 10.88.88.2

### VPS relay (container wg-relay)
- Priv: QCt+0amrJr1J4Zdc1sxiwbrthzFEfkyG807A7HnK8ns=
- Pub: jLMHG2nDzVgnzcHfBvE1Towgq77zVwJOVrRF7ZQ5+z8=
- IP: 10.88.88.1, port 51830
- DNAT+MASQUERADE port 51820 → bastion 10.88.88.2:51820

## VPS (89.234.141.181)

- User SSH: docker (clé dans ~/backup-windows/ssh/id_ed25519)
- Container wg-relay: port 51830 (tunnel) + 51820 (forward vers bastion)
- Config: /home/docker/wg-relay/wg_confs/wg0.conf

## PureVPN

- User: purevpn0s14050084
- Pass: c8UvId4pPZMUn5
- CH: 188.240.60.73:15021 UDP (tunnel système)
- NL: 149.40.59.70:15021 UDP (sortie)
- Auth file: /etc/openvpn/purevpn-auth.txt

## Cloudflare Tunnel

- Tunnel ID: 8c9c75ae-4dc0-478d-9dc5-ac89725e6695
- Tunnel name: vpnplex
- Credentials: /etc/cloudflared/8c9c75ae-4dc0-478d-9dc5-ac89725e6695.json
- Config: /etc/cloudflared/config.yml
- Domaines: vpn.appstorefr.net (web admin), ssh.appstorefr.net (SSH)
- Account: appstorefr@ik.me, Account ID: 92f26b656dbc5fa6073624b68e93c3ca
- API Key: 85f80411f7a979c344bc142c155616c462f34
- Zone ID appstorefr.net: 782fa2761584c82aea2fb52791be61b9

## Interface web VPNPlex

- URL locale: http://192.168.0.20:8080
- URL publique: https://vpn.appstorefr.net (via Cloudflare Tunnel)
- Auth: freelux / tgboulet (HTTP Basic)
- App: /opt/vpnplex/web/app.py (Flask natif)
- DB: /opt/vpnplex/data/vpnplex.db
- Templates: /opt/vpnplex/web/templates/
- Fonctions: gestion peers WG (QR code, config texte, download .conf), ajout sorties VPN (upload .ovpn ou .conf WG auto), choix sortie par peer, start/stop
- Page publique par peer: /c/<NomDuPeer> (QR + download, sans auth)
- Sorties système (non suppressibles): VPS Direct (wg-relay), Suisse système (tun0)

## AdGuard Home

- URL: http://192.168.0.20:3000
- Auth: freelux / tgboulet
- Upstreams: Cloudflare DoH, NextDNS QUIC, AdGuard DNS QUIC
- DNSSEC activé, IPv6 AAAA désactivé, cache optimistic
- Filtres: AdGuard DNS filter + OISD Small
- Whitelist: AliExpress
- Services bloqués: Tinder, POF, Wizz
- Logs: 24h max, pas de fichier
- Config: /opt/AdGuardHome/AdGuardHome.yaml

## Sécurité parano

- Kill switch: /opt/vpnplex/killswitch.sh (service vpnplex-killswitch)
  - DROP 10.77.77.0/24 → enp2s0f0 (peers WG)
  - DROP 192.168.77.0/24 → enp2s0f0 (futur LAN USB-Ethernet)
- Anti DNS leak: DNAT port 53 des subnets VPN vers AdGuard Home local
- DNS chiffré: upstreams DoH/QUIC uniquement
- WireGuard ne log rien par défaut
- Cloudflare voit IP PureVPN CH, pas IP maison

## Claude Code

- Installé sur le bastion: claude v2.1.76
- Alias: goclaude = claude --dangerously-skip-permissions
- tmux installé pour sessions persistantes
- Usage: `ssh bastion` → `tmux new -s claude` → `goclaude`

## UFW

- 22/tcp (SSH), 2222/tcp (Dropbear), 51820/udp (WG), 8080/tcp (Web), 3000/tcp (AdGuard), 53 on wg0 (DNS)
- Route: wg-relay↔wg0, wg0↔tun0, wg0↔tun1, wg0↔wg-relay
- fail2ban actif

## Fichiers importants

- /etc/wireguard/wg0.conf — config WG serveur
- /etc/wireguard/wg-relay.conf — tunnel relay vers VPS
- /etc/openvpn/purevpn-ch.conf — tunnel système CH
- /etc/openvpn/purevpn-nl.conf — sortie NL
- /opt/vpnplex/ — web app, data, exits
- /opt/vpnplex/killswitch.sh — kill switch + anti DNS leak
- /etc/cloudflared/config.yml — config tunnel Cloudflare
- /opt/AdGuardHome/AdGuardHome.yaml — config AdGuard
- ~/unlock-bastion.sh — script unlock LUKS distant (sur PC principal)
- ~/fix-ssh-bastion.sh — script fix SSH (sur PC principal)

## Statut (2026-03-24)

- **Bastion HS** — SSH via Cloudflare Tunnel ne fonctionne plus ("websocket: bad handshake"), état inconnu
- Le rôle VPN cascade est en cours de migration vers le PC Xubuntu (192.168.0.4) — voir infra_xubuntu.md
- Les credentials PureVPN et l'architecture restent les mêmes

## TODO

- Diagnostiquer/fixer le bastion ou le décommissionner
- Récupérer les certificats CA PureVPN (dans les .conf du bastion si accessible)

**Why:** VPN self-hosted parano max, IP maison invisible, sorties VPN commerciales interchangeables, serveur Claude Code distant H24
**How to apply:** Toute modif réseau/VPN doit respecter l'architecture (ne jamais exposer l'IP maison). Ne jamais faire de test full tunnel depuis le PC principal (ça coupe le SSH). Le VPN cascade migre vers le Xubuntu.
