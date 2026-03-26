---
name: Projet TiviMate V2
description: TiviMate panel multi-users, password backup cracké, génération APK/TMB, CF Worker + bastion
type: project
---

## Dossier principal
`/home/freelux/projets/tivimate/`

## PASSWORD BACKUP CRACKÉ (2026-03-20)

```
Hex: 02 30 62 30 60 0d 16 19 37 2a 2f 02 62 06 04 01 1c 0d 02 01 40 02 04 6c 04 37 06
```
- 27 chars dont beaucoup non-imprimables (0x02, 0x0d, 0x16...)
- Constant pour la version moddée RTX+AndyHax 5.1.6
- Validé: `7z t -p"$(python3 -c "print(''.join(chr(c) for c in [0x02,0x30,0x62...]))")" backup.tmb` → Everything is Ok
- **Méthode**: patch smali du constructeur `ⁱʼ/ـﹶ.<init>(File, char[])` → Log.e("TIVI_PWD", hex)
- DexProtector ne détecte pas car c'est du code interne, pas de l'instrumentation externe

## Versions APK

| Fichier | Taille | Description |
|---------|--------|-------------|
| `apk/original/tivimate_5.2.08_clean.apk` | 17 Mo | TiviMate officiel, clean, pas de mod |
| `apk/modded/tivimate_5.1.6_rtx_andyhax.apk` | 57 Mo | Moddé RTX+AndyHax, premium cracké, libhax.so/liborigin.so |
| `apk/modded/tivimate_5.1.6_rtx_aligned.apk` | 81 Mo | Moddé aligned pour decompilation baksmali |
| `apk/patched/tivimate_patched_fixedpwd.apk` | 98 Mo | Password backup remplacé par un fixe connu |
| `apk/patched/tivimate_patched_pwdlogger.apk` | 57 Mo | Log password hex via Log.e (celui qui a cracké) |
| `apk/debug/tivimate_v1_debug.apk` | 71 Mo | Build debug instrumenté (session Windows) |
| `tivimate_5.1.6_with_panel_original.zip` | 55 Mo | ZIP original AndyHax: APK + panel PHP + Docker |

## Architecture backup .tmb
- ZIP AES chiffré via zip4j (net.lingala.zip4j)
- Contenu: preferences XML, playlist categories XML, TvPlayer.db (+shm/wal)
- Password généré par méthode native `Lˎʼ/ˋˉ;->ᐧʻ()[C` (DexProtector)
- Appelé dans `SyncRecordingsWorker.smali` (lignes 1284, 1490)

## Protections DexProtector
- **Détecte**: Frida (ptrace/maps), JDWP, Xposed/LSPosed, émulateur QEMU
- **Ne détecte PAS**: patch smali interne à l'APK
- `libdexprotector.so` (356 Ko) = loader, déchiffre payload "DPLF" dans l'ELF
- `liborigin.so` (12 Mo) = code natif complet version moddée (contient DEX chiffrés)
- La classe `Lˎʼ/ˋˉ;` n'existe PAS en smali — entièrement native

## Historique complet

### V0 — Panel PHP+Docker (AndyHax original)
- Stack: PHP + Docker + SQLite
- Fichiers: index.php, main.php, user.php, welcome.php, note.php, api/
- Frontend: Vanta.js (three.js), CSS custom
- **Rangé dans**: `panel/v0-andyhax-php/`
- **Status**: jamais déployé, base de référence

### V1 — Panel CF Worker (session Windows, ~18-19 mars 2026)
- Développé depuis PC Windows (Gaming-Corsair)
- Stack: CF Workers + D1 (SQLite) + KV sessions
- Code: `panel/cf-worker/` (index.js, routes/auth.js, panel.js, admin.js, api.js)
- Wrangler config: BASTION_URL → 192.168.0.20:5050
- Schema D1: users, playlists, epg, configs
- Backup hébergé testé sur `tivimate.appstorefr.net/custom_backup.tmb`
- **Status**: code écrit, partiellement testé, pas en prod

### V1 — Script ADB (session Windows)
- `scripts/adb_tivimate.sh` — inject playlists directement dans DB Room via run-as
- Contrôle box TV: screenshot, navigation D-pad, permissions bypass
- Backup restore fichier local: testé OK
- Backup restore via URL: hébergé sur CF, test non finalisé

### V1 — APK debug/instrumentés (session Windows)
- `tivimate_v1_debug.apk` (71 Mo) — première tentative instrumentation
- `TiviMate_v2_tivi2.apk` (71 Mo) — deuxième itération
- Frida sur box Android TV (localhost:5555) — bloqué par DexProtector
- Bruteforce password backup (rockyou, hex, digits, alpha) — échec total

### V1-crack — Extraction password (session Linux/Poco, 2026-03-20)
- **Poco M3 Pro 5G** flashé: MIUI stock → crDroid v9 Android 13 + Magisk 30.7 + LSPosed Zygisk
- Flash via mtkclient (BROM) car fastboot USB freezait à 12% (câble défectueux)
- Tentatives: Frida → kill, JDWP → kill, Xposed module custom (buildé from scratch) → kill
- **Solution**: patch smali constructeur ZIP `ⁱʼ/ـﹶ` pour logger char[] en hex
- Rebuild APK: baksmali → smali assemble → replace DEX dans APK → apksigner v2
- **Résultat**: password loggé dans logcat tag "TIVI_PWD", validé sur 7z
- Outils installés sur Poco: Magisk, LSPosed, Frida server, LLM4Decompile (Docker GPU)

### V2 — Panel multi-users + APK generator (PLANIFIÉ, pas commencé)
- CF Worker sur `tivi.appstorefr.net/config`
- Users créent comptes, configurent playlists, EPG
- Génération APK custom à la volée (nom, logo, endpoint API)
- Génération .tmb chiffré avec le password cracké (7z AES)
- Backend API sur bastion (Flask/FastAPI)
- Hébergement APK/TMB sur R2 (Cloudflare)

## Outils & infra

### Scripts
- `scripts/adb_tivimate.sh` — inject playlists via ADB + run-as
- `scripts/tivi_perms.sh` — autorise superposition + stockage sur Poco
- `scripts/bruteforce_tmb.sh` — bruteforce historique (plus utile)

### Poco M3 Pro 5G (device de test)
- crDroid v9, Magisk 30.7, LSPosed, root, ADB serial: 5dqsssjbfyfyciem
- Bootloader déverrouillé (NE PAS reverrouiller)
- Détails complets dans `project_poco_crdroid.md`

### Analyse reverse engineering
- `analysis/reports/` — rapport sécu, RTX decompilé (Ghidra), JNI, strings
- `analysis/liborigin.so` — lib native 12 Mo de la version moddée
- `analysis/session_windows_*.txt` — archives des sessions Claude sous Windows
- Ghidra project: `/tmp/ghidra_tivi/` (libdexprotector analysée)

### DB
- `db/TvPlayer.db` — base Room SQLite TiviMate (5.1 Mo, user version 55)

## Fichiers Windows à supprimer
Listés dans `WINDOWS_FILES_TO_DELETE.md` (~350 Mo récupérables)
Sessions archivées dans `analysis/session_windows_*.txt`

**Why:** Créer un service SaaS pour déployer des configs TiviMate à distance via restore URL, sans ADB.
**How to apply:** Password cracké, panel V1 existe, V2 à développer avec APK generator et TMB generator.
