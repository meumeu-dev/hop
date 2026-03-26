---
name: Project QEMU VM
description: Setup QEMU/KVM avec mode parano (network namespace VPN), virt-manager, en cours
type: project
---

- QEMU/KVM + virt-manager installés et fonctionnels sur le PC
- Script `vm-parano` créé dans `/usr/local/bin/vm-parano` — lance QEMU dans un network namespace isolé avec WireGuard
- Config WireGuard client existante : `/etc/wireguard/vpnplex-test.conf`
- User ajouté aux groupes libvirt et kvm

**Reste à faire :**
- Configurer un réseau libvirt isolé qui route via WireGuard (pour utiliser virt-manager avec isolation VPN)
- Interface web WireGuard (wg-easy) — à déployer sur le bastion ou une VM, PAS sur le PC
- Télécharger ISO Windows + drivers VirtIO
- Créer la VM Windows

**Why:** L'utilisateur veut pouvoir lancer des VMs Windows isolées dont tout le trafic passe par VPN, sans fuite possible.
**How to apply:** Ne RIEN déployer côté réseau sur le PC directement. Proposer bastion ou VM pour les services réseau.