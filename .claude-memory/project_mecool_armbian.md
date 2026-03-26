---
name: Mecool KM1 Armbian + AdGuard
description: Flash Armbian sur Mecool KM1 Collective Edition (S905X3) pour serveur AdGuard Home — actuellement briquée après install eMMC
type: project
---

## Objectif
Transformer une Mecool KM1 Collective Edition en serveur AdGuard Home + Docker + aria2 + WireGuard + CF Tunnel + Homepage.

## Hardware
- **Box** : Mecool KM1 Collective Edition
- **SoC** : Amlogic S905X3 (ARM64, 4x Cortex-A55 @ 2.0 GHz)
- **RAM** : 940 Mo
- **eMMC** : 58 Go
- **DTB recommandé** : `meson-sm1-sei610.dtb`
- **Ports USB** : 1 noir (USB 2.0), 1 bleu (USB 3.0)
- **Réseau** : Gigabit Ethernet

## Ce qui a été fait (2026-03-19)
1. Image Armbian S905X3 server (kernel 6.12.77 LTS) flashée sur clé USB 16 Go
2. DTB configuré sur `meson-sm1-sei610.dtb` dans `uEnv.txt`
3. Boot USB OK, SSH fonctionnel (IP 10.0.0.224)
4. Comptes créés : root / `FTGb0ulet@69++`, freelux / `tgboulet` (sudo)
5. AdGuard Home installé et fonctionnel (:3000)
6. `armbian-install` exécuté sur eMMC avec model DB patchée (501 → sei610)
7. Installation eMMC réussie (message SUCCESS)
8. Après retrait clé USB et reboot → **box ne boot plus** (pas de LED, pas de réseau, pas de HDMI)
9. Un ping bref a été capté une fois → le kernel démarre mais crash (probablement DTB)

## Statut : BRIQUÉE
- Le bouton reset (trou AV) ne force pas le boot USB
- USB Burning mode non détecté via USB-A ou USB-C
- Prochaine étape : **Amlogic USB Burning Tool sous Windows** ou **court-circuit eMMC** sur le PCB

## Plan de services prévu
- AdGuard Home (~25 Mo RAM)
- Docker (~30 Mo)
- WireGuard (~5 Mo)
- Cloudflared tunnel (~20 Mo)
- aria2 + AriaNg (~10 Mo) — download manager léger
- Filebrowser (~15 Mo)
- Homepage (~30 Mo)

**Why:** Besoin d'un serveur DNS/adblock + services légers sur le réseau local.
**How to apply:** Débriquer d'abord via Windows USB Burning Tool, puis repartir avec Armbian en vérifiant le bon DTB avant install eMMC.
