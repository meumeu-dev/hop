---
name: Poco M3 Pro 5G - crDroid + TiviMate password hunt
description: Poco M3 Pro 5G flashé crDroid v9 A13, Magisk root, LSPosed Zygisk, objectif extraire password backup TiviMate
type: project
---

## État du Poco M3 Pro 5G (camellian)
- **ROM**: crDroid v9 officiel (Android 13) - boot OK
- **Root**: Magisk 30.7 (Zygisk activé)
- **LSPosed**: v1.9.2-7024 Zygisk installé (SEPolicy warning mais fonctionnel)
- **Bootloader**: déverrouillé (NE PAS reverrouiller avec ROM custom)
- **Serial ADB**: 5dqsssjbfyfyciem
- **Firmware**: MIUI A13 stock (V14.0.6.0.TKSMIXM) flashé via mtkclient
- **Câble USB**: l'ancien câble freeze à 12% sur les gros transfers, le nouveau marche

## Fichiers importants
- `/home/freelux/poco/` — dossier de travail
- `/home/freelux/poco/crdroid_boot.img` — boot recovery crDroid
- `/home/freelux/poco/magisk_patched_boot.img` — boot Magisk patché
- `/home/freelux/poco/backup.tmb` — backup TiviMate chiffré (ZIP AES, 9Ko)
- `/home/freelux/poco/liborigin.so` — lib native de la version moddée RTX (12Mo)
- `/home/freelux/poco/all_rw.bin` — dump mémoire RW du process TiviMate
- `/home/freelux/poco/dalvik_post_backup.bin` — dump dalvik heap post-backup (128Mo)
- `/home/freelux/crdroid_camellia.zip` — ROM crDroid
- `/home/freelux/backup-windows/apk/tivimate.apk` — TiviMate 5.2.08 clean original (17Mo)
- `/home/freelux/backup-windows/apk/TiviMate_5.1.6.apk` — version moddée RTX+AndyHax (57Mo)
- `/tmp/tivi_clean/lib/arm64-v8a/libdexprotector.so` — lib DexProtector clean (356Ko)
- `/tmp/ghidra_tivi/` — projet Ghidra avec libdexprotector analysée

## Tentatives d'extraction du password

### Ce qui a été tenté et ÉCHOUÉ
1. **Frida attach** — DexProtector détecte ptrace/Frida maps, kill le process
2. **Frida spawn** — même résultat, crash avant que les libs natives se chargent
3. **JDWP/jdb** — DexProtector détecte le debugger, kill
4. **Xposed/LSPosed module** — hook s'installe (`[TiviHook] Hook installed!`) mais DexProtector détecte le framework Xposed et crash avec `MessageGuardException` code 789
5. **Memory dump strings** — password pas trouvé dans heap Java ni segments RW (calculé à la volée dans registres CPU, jamais stocké comme string)
6. **Ghidra statique libdexprotector.so** — c'est juste un loader qui déchiffre un payload caché dans l'ELF (signature "DPLF"), le vrai code est chiffré
7. **Bruteforce** — déjà tenté avant, rien trouvé (rockyou, hex, digits, alpha)
8. **SharedPrefs** — `com.andyhax.cache.xml` contient un gros blob hex mais pas le password en clair

### Ce qui POURRAIT marcher (à explorer)
1. **Unicorn Engine** — émuler les fonctions ARM64 de libdexprotector pour exécuter i(int) isolément
2. **LD_PRELOAD hook** — intercepter les appels JNI (RegisterNatives, NewCharArray) via preload
3. **Kernel module / kprobe** — intercepter au niveau kernel les appels système
4. **Ghidra sur liborigin.so (12Mo, version moddée)** — plus grosse, peut contenir le password en clair
5. **Émulateur avec anti-detection bypass** — Waydroid ou autre avec patches anti-QEMU
6. **Known-plaintext attack** — le .tmb contient des XML/SQLite dont on connaît la structure
7. **Patch binaire libdexprotector** — désactiver les checks anti-debug dans le loader

### Architecture DexProtector
- `libdpboot.so` (8Ko) — bootstrap, charge libdexprotector
- `libdexprotector.so` (356Ko) — loader, déchiffre payload "DPLF" caché dans l'ELF
- Les méthodes `i(int)` sont enregistrées dynamiquement via `RegisterNatives` dans le code déchiffré
- Le password est un `char[27]` construit char par char via `LibTvPlayerApplication.i(INDEX)` avec des indices DexProtector
- Indices: `[0x38, 0x6e, 0x7f, 0x6e, 0x114, 0xb3, 0x350, 0x282, 0x9d, 0x357, 0x359, 0x38, 0x7f, 0x6c, 0x51, 0x5d, 0x34a, 0xb3, 0x38, 0x5d, 0x2e8, 0x38, 0x51, 0x1cd, 0x51, 0x9d, 0x6c]`
- Détections DexProtector: ptrace, /proc/self/maps (Frida), JDWP, Xposed framework, émulateur QEMU

**Why:** Extraire le password des backups .tmb pour pouvoir créer/modifier des backups programmatiquement dans le panel CF Worker.
**How to apply:** Toute tentative d'instrumentation runtime est détectée. Privilégier l'analyse statique, l'émulation CPU, ou le contournement au niveau kernel/système.
