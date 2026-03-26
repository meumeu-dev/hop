---
name: Migration bastion VPNPlex
description: Migration complete PC principal vers bastion (192.168.0.20) le 2026-03-16 - 21 projets, yolobox, secrets centralises
type: project
---

Migration effectuee le 2026-03-16 depuis PC principal (Ubuntu 24.04) + backup Windows vers bastion Debian 13 (192.168.0.20).

## Ce qui a ete migre
- 21 projets actifs dans ~/dev/ (1.3 Go) avec .yolobox.toml + CLAUDE.md chacun
- 7 archives dans ~/archive/ (3.3 Go)
- 11 fichiers secrets dans ~/secrets/ (chmod 700/600)
- Credentials + settings Claude Code
- 21 memoires Claude (16 projets + 3 feedback + 1 user + 1 infra) avec chemins mis a jour
- 6 nouvelles memoires integrees depuis backup Windows (unified, ghost, lutins, gestionchiffre, vpnconnect, pdm)

## Organisation bastion
- ~/dev/ : 1 dossier = 1 projet = 1 sandbox yolobox
- ~/archive/ : stockage froid (reference)
- ~/secrets/ : credentials centralises, jamais montes en entier dans yolobox
- ~/yolo.sh : helper tmux + yolobox claude

## Outils installes
- yolobox v0.10.0, tmux 3.5a, Docker 29.3.0, rsync
- Config yolobox globale: 2 CPU, 2Go RAM, git+ssh+gh, copy_agent_instructions

## Securite
- aquatree: chmod 700 + no_network=true
- Aucun .env dans les projets (centralises dans ~/secrets/env-files/)
- Secrets: 700 dossiers, 600 fichiers

**Why:** Centraliser tous les projets sur le bastion pour acces Claude Code 24/7 via SSH, isoles par yolobox
**How to apply:** Travailler sur le bastion via `ssh bastion` puis `./yolo.sh <projet>`. Ne plus modifier les sources sur le PC principal.
