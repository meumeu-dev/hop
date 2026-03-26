---
name: Disk Tests Results
description: Resultats tests badblocks sur 2 disques 1TB - les deux sains, GPT vide prets a l'emploi
type: project
---

Tests effectues le 2026-03-12/13:

Disque 1: Inconnu (boitier Zalman ZM-VE200), 1TB, ancien NTFS label "Backup"
- Badblocks: 0 erreurs, sain
- Efface (GPT vide)

Disque 2: WD WD10SPZX (WD Blue 2.5"), serie WD-WXB1A28CC4U4, 1TB
- Ancien contenu: Linux install (user "paco")
- Backup perso sauvegarde: ~/backup-windows/paco-backup.tar.gz (371MB)
- Badblocks: 0 erreurs, sain
- Efface (GPT vide)

Projet futur: machine Debian pour recuperation de donnees (testdisk, ddrescue, photorec)
Methode: toujours cloner d'abord avec ddrescue, travailler sur le clone en read-only
