---
name: Fusion WebADB + RemoteADB — ADB Mix
description: Projet adb-mix fusionnant WebADB + RemoteADB dans ~/meumeudev/adb-mix/ — deploye sur webadb.appstorefr.net, Worker CF, agent Go, APK Android, page admin licences
type: project
---

## ADB Mix — Etat actuel (2026-03-13)

**Why:** Unifier WebADB (local) + RemoteADB (distant) en un seul projet. A terme, supprimer radb.appstorefr.net et les anciens projets.

### Deploiement LIVE

- **Site web** : `https://webadb.appstorefr.net/` — sert l'app directement (WebUSB, Agent, Extension sans code)
- **Remote** : `https://webadb.appstorefr.net/{CODE}/` — app pre-configuree pour tunnel remote
- **Admin** : `https://webadb.appstorefr.net/admin/` — Basic Auth `freelux:tgboulet`
- **Downloads** : `https://webadb.appstorefr.net/download/{fichier}`
- **Compte CF** : appstorefr@ik.me (credentials dans `/home/freelux/appstorefr/srv/token/cloudflare-api.txt`)
- **Account ID CF** : `92f26b656dbc5fa6073624b68e93c3ca`
- **Zone** : `782fa2761584c82aea2fb52791be61b9` (appstorefr.net)

### Cloudflare Worker (`worker/`)

- **Nom** : `adbmix-worker`
- **KV namespace** : `ADBMIX_KV` (ID `0c8d5156ce694a88b12f2dc0c7673255`)
- **TUNNEL_SECRET** : `adbmix_tunnel_2024`
- **DNS** : `webadb.appstorefr.net` AAAA 100:: proxied
- **KV prefixes** : `webapp:` (frontend), `tunnel:` (tunnels actifs), `license:` (licences), `download:` (binaires)
- **Routes** : `/tunnel/register|unregister`, `/license/validate`, `/admin/`, `/download/`, `/{CODE}/` (proxy), `/` (app)
- **Deploy** : `cd worker && CLOUDFLARE_EMAIL=... CLOUDFLARE_API_KEY=... npx wrangler deploy`
- **Upload frontend** : `bash deploy-webapp.sh`

### Agent Go (`agent/`)

- Version 1.1.0, sert le frontend en statique sur `/` (auto-detect `../frontend/` ou flag `--frontend`)
- Tunnel cloudflared : code word-based (127 mots x2, ex: "acefox")
- Auth JWT quand tunnel actif (localhost exempt)
- Routes specifiques : `/api/tunnel/start|stop|status`, `/api/auth/pair|verify`, `/api/packages/install-url`, `/api/scripts`
- Build : `cd agent && make all` (6 cibles cross-compile)

### APK Android (`android/`)

- **Taille** : 6.6 MB (vs 24-52 MB avant)
- **Webapp** : supprimee de l'APK, servie par le Worker
- **cloudflared** : supprime du APK (jniLibs vire), telecharge au 1er lancement depuis `webadb.appstorefr.net/download/libcloudflared-{arch}.so.gz` (gzippe), cache dans filesDir
- **Licences** : systeme de licences ACTIF, valide contre `webadb.appstorefr.net/license/validate`
- **AppConfig defaults** : `radbBaseUrl = "https://webadb.appstorefr.net"`, `tunnelSecret = "adbmix_tunnel_2024"`
- **Build** : `ANDROID_HOME=~/android-sdk JAVA_HOME=/usr/lib/jvm/java-17-openjdk-amd64 ./gradlew assembleRelease`
- **SDK** installe dans `~/android-sdk/` (platform-34, build-tools-34.0.0)

### Extension navigateur (`extension/`)

- Chrome (manifest v3) + Firefox (manifest-firefox.json)
- Packagee dans `agent/build/adbmix-extension-chrome.zip` et `adbmix-extension-firefox.xpi`

### Frontend (`frontend/`)

- SPA Vue 3 sans build (CDN), 5 transports : Agent, WebUSB, Extension, Docker, Remote
- Page Connexion : section remote + section tunnel sharing + section telechargements (agent/extension/APK) + push APK sur device
- ScreenView : dual mode (screenshot polling local / scrcpy H.264 remote)
- ScriptsView : visible uniquement en remote

### Fichiers uploades dans KV (downloads)

- `webadb-agent-{os}-{arch}[.exe]` (6 binaires ~7MB)
- `adbmix-extension-chrome.zip`, `adbmix-extension-firefox.xpi`
- `adbmix-remoteadb.apk` (6.6MB, universel)
- `libcloudflared-{arm64-v8a,armeabi-v7a,x86_64}.so.gz` (9-19MB compresse)
- Anciens APK per-arch encore presents : `remoteadb-arm64.apk`, `remoteadb-armv7.apk` (a nettoyer)

### Points en suspens

- Le bouton "Pousser l'APK RemoteADB" a maintenant un fallback (install-url → download+upload multipart), mais necessite que l'agent Go tourne sur 8800, pas un backend Node. Le message d'erreur guide l'utilisateur.
- Pas de reference restante a `radb.appstorefr.net` dans le code — pret a supprimer l'ancien worker
- Anciens APK per-arch dans KV a nettoyer
