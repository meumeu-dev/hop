---
name: Infra Xubuntu 192.168.0.4
description: PC Xubuntu i7-4790K 16Go, héberge VMs agents, RustDesk, VPN cascade prévu, KVM/QEMU installé
type: project
---

## PC Xubuntu — 192.168.0.4

**Hardware:** i7-4790K, 16 Go RAM, 1.8 To disque, vieille CG
**OS:** Xubuntu 24.04 Noble
**User:** freelux / tgboulet
**SSH:** sshpass -p 'tgboulet' ssh freelux@192.168.0.4
**Rôle:** Serveur VMs agents IA, allumé H24

## RustDesk (remplace VNC)
- Installé sur Xubuntu + PC principal
- RustDesk ID Xubuntu: 178525728
- RustDesk ID PC principal: 1750271065
- Password: tgboulet
- Raccourci bureau PC principal: ~/Bureau/RustDesk-PC-Xubuntu.desktop
- Raccourci bureau Xubuntu: ~/Desktop/RustDesk-PC-Principal.desktop
- VNC désinstallé des deux côtés (sauf tigervnc-viewer sur PC principal pour consoles VM)

## KVM/QEMU
- qemu-kvm, libvirt, virt-manager, virtinst installés
- Réseau isolé "isolated": virbr1, 10.10.10.0/24 (DHCP 10.10.10.10-50), pas d'accès internet
- VM agent-01: Lubuntu 24.04, 4Go RAM, 2 vCPU, 50Go disque, LUKS, réseau isolé — install en cours via ISO

## NetworkManager
- enp2s0 géré par NM, connexion "Filaire" active
- systemd-networkd désactivé
- Override: /etc/NetworkManager/conf.d/99-managed.conf (managed=true)

## VPN cascade (en cours de setup, 2026-03-24)
- OpenVPN + WireGuard installés
- PureVPN auth: /etc/openvpn/purevpn-auth.txt
- Config VPN1 (CH): /etc/openvpn/purevpn-ch.conf — MANQUE certificats CA PureVPN
- Config VPN2 (NL): /etc/openvpn/purevpn-nl.conf — MANQUE certificats CA PureVPN
- AdGuard Home: installé dans /opt/AdGuardHome/, pas encore configuré
- Architecture: VMs (10.10.10.0/24) → VPN1 (CH) → VPN2 (NL) → Internet
- Le host Xubuntu NE passe PAS par le VPN (policy routing, seules les VMs)
- AdGuard Home servira le DNS pour le host ET les VMs
- Kill switch: DROP trafic VMs si VPN down

## Openclaw
- Installé (/usr/bin/openclaw), pas de profil ~/.openclaw/
- Plan: migrer profil depuis PC principal, Xubuntu devient gateway, PC principal devient node
- Pas encore fait

## TODO
- Récupérer certificats CA PureVPN (bastion HS, chercher en local ou re-télécharger)
- Finir config VPN cascade + kill switch + policy routing
- Configurer AdGuard Home
- Finir install VM agent-01 (Lubuntu via ISO, LUKS)
- Migrer profil openclaw

**Why:** Centraliser les agents IA autonomes sur machine H24, isoler le trafic VMs via VPN cascade parano (IP maison jamais exposée)
**How to apply:** Ne JAMAIS router le trafic du host Xubuntu via VPN. Seul le subnet VMs (10.10.10.0/24) passe par la cascade.
