---
name: Debian Preseed USB Multiboot
description: Cle USB multiboot GPT/GRUB BIOS+UEFI, Debian preseed LUKS+dropbear, live, SystemRescue, Clonezilla, memtest, network-console SSH pour pilotage Claude
type: project
---

## Cle USB Multiboot Toolbox (Cruzer Facet 7.5G)

**Why:** Clef technicien tout-en-un: install auto serveur, rescue, clonage, diagnostics, pilotage install a distance via Claude Code.

### Structure USB (GPT)
- Partition 1: BIOS Boot (1MB, ef02)
- Partition 2: EFI System (100MB FAT32, label EFI)
- Partition 3: Données (7.4GB FAT32, label MULTIBOOT)
- GRUB installe en dual BIOS + UEFI

### Contenu /iso/
| ISO | Taille |
|-----|--------|
| debian-13.3-preseed-netinst.iso | 940 MB |
| debian-live-13.4.0-amd64-standard.iso | 1.9 GB |
| systemrescue-12.03-amd64.iso | 1.2 GB |
| clonezilla-live-3.3.1-35-amd64.iso | 370 MB |

### Menu GRUB (loopback boot des ISOs)
1. **[DEBIAN] Auto-Install preseed** - LUKS+LVM+SSH+dropbear, FR/azerty
2. **[DEBIAN] Install manuelle** - netinstall classique
3. **[DEBIAN] Install SSH** - network-console activé (ssh installer@IP, pass: temppass)
4. **[LIVE] Debian Live 13.4** - standard console+réseau
5. **[RESCUE] SystemRescue 12.03** - gparted, testdisk, photorec, chroot
6. **[CLONE] Clonezilla 3.3.1** - clonage/backup disques
7. **[DIAG] Memtest86+** - BIOS + EFI
8. Boot HDD / Reboot / Shutdown / UEFI Setup

### Config preseed (dans l'ISO netinstall)
- Debian 13.3 Trixie, locale fr_FR.UTF-8, azerty pc105, DHCP
- LUKS (passphrase: `salefilsdepute`) + LVM (vg0) disque entier
- User: freelux / tgboulet, sudo NOPASSWD
- SSH port 22 + Dropbear port 2222 (initramfs, unlock LUKS distant)
- Network-console: ssh installer@IP (pass: temppass) pour pilotage a distance
- Clavier azerty dans initramfs/dropbear (KEYMAP=y + hook custom)
- hostname.sh installe dans /usr/local/bin/ apres install

### Scripts sur la clé (/scripts/)
- `hostname.sh <newhostname>` - change hostname proprement (hostnamectl, /etc/hosts, initramfs)
- `live-ssh-start.sh [password]` - active SSH en live pour pilotage Claude Code a distance
- `preseed.cfg` - copie de reference

### Pilotage install a distance via Claude Code
1. Boot "[LIVE] Debian Live" ou "[RESCUE] SystemRescue"
2. Monter MULTIBOOT, lancer `live-ssh-start.sh`
3. Depuis PC principal: `claude "SSH root@<IP>, installe Debian..."`

### Deverrouillage LUKS a distance
```bash
ssh -p 2222 root@IP_SERVEUR
cryptroot-unlock
# taper passphrase: salefilsdepute
```

### Espace restant: ~3.1 Go (pour ajouter d'autres ISOs)

### Fichiers sources builds
- ISO rebuild workspace: `/tmp/usb-multiboot/`
- GRUB config: `/tmp/usb-multiboot/grub.cfg`
- Preseed: `/tmp/usb-multiboot/work/preseed.cfg`
