---
name: User Profile
description: Profil utilisateur freelux - dev/admin sys, reseau 10.0.0.x, routeur OpenWrt, projets AI/web/hardware
type: user
---

- Nom: Meunier, surnom "meumeu", lance une societe depannage info + dev web/IA
- Domaine: meumeu.dev (Cloudflare, landing page CF Pages)
- Utilisateur Linux (Ubuntu 24.04), dev et admin sys
- PC principal: 32 Go RAM, RTX 2080, Ubuntu 24.04 (display :1)
- PC Xubuntu: 192.168.0.4, i7-4790K, 16 Go RAM, Xubuntu 24.04, allumé H24, héberge VMs agents
- Reseau local 192.168.0.x (Livebox), routeur OpenWrt AX6S (192.168.0.10 coté LAN Livebox, 10.0.0.1 coté WiFi/LAN OpenWrt, SSH via cle ed25519 copiee dans ~/.ssh/)
  - WiFi: FamilleMeuMeu (sae-mixed, 802.11r), Guest, IoT (VLANs 1/10/20)
  - AdGuard Home pour filtrage DNS
  - Cloudflare DNS redirection (port 53 DNAT)
- Partition Windows montee read-only sur /mnt/windows (user Corsair)
- GPU: RTX 2080 8GB, Ollama + Docker AI stack dessus
- Prefere les solutions pragmatiques, pas de sur-ingenierie
- Francophone, communication directe et informelle
- Alias bash: goclaude = claude --dangerously-skip-permissions
