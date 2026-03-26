---
name: WebADB Project
description: Projet WebADB - interface web ADB hybride (WebUSB/Agent Go/Extension navigateur) dans ~/meumeudev/webadb/
type: project
---

## WebADB - Interface Web ADB

**Emplacement:** `/home/freelux/meumeudev/webadb/`

**Why:** Permettre le controle d'appareils Android via ADB depuis un navigateur, hebergeable en statique pour que chaque utilisateur puisse l'utiliser chez lui.

**How to apply:** Le projet a 4 composants independants, tous fonctionnels.

### Architecture

```
meumeudev/webadb/
├── frontend/              # SPA Vue 3 statique, dark theme (style RemoteADB)
│   ├── assets/app.js      # ~1200 lignes, composants: Dashboard, Connect, Terminal, Screen, Packages, Files, Logcat, Processes, Settings
│   ├── assets/style.css
│   └── assets/transports/  # Couche abstraction: agent-transport.js, webusb-transport.js, extension-transport.js, transport.js (auto-detection)
├── agent/                 # Agent Go portable (bridge API ADB sur localhost:8800)
│   ├── main.go + internal/{adb,server,discovery}/
│   ├── Makefile           # Cross-compile: make all
│   └── build/             # 6 binaires: linux/windows/darwin x amd64/arm64 (~7MB chacun)
├── extension/             # Extension Chrome (MV3) + Firefox
│   ├── manifest.json      # Chrome
│   ├── manifest-firefox.json
│   ├── background/        # Agent detection + native messaging
│   ├── content/           # Bridge postMessage page <-> extension
│   ├── popup/             # Mini UI dark theme
│   └── native-host/       # Install scripts native messaging (Linux/Mac/Windows)
├── backend/               # Ancien backend Node.js (Docker)
│   └── server.js          # Express + WebSocket, sert aussi le frontend
├── Dockerfile             # Node.js Alpine + android-tools + util-linux + avahi-tools
└── docker-compose.yml     # network_mode: host, privileged, port 8800
```

### Modes de fonctionnement

| Mode | USB | WiFi ADB | Scan reseau | Install requise |
|------|-----|----------|-------------|-----------------|
| WebUSB | oui | non | non | aucune (Chrome/Edge) |
| Agent Go | oui | oui | oui | telecharger 1 binaire |
| Extension + Agent | oui | oui | oui | extension + agent |
| Docker | oui | oui | oui | docker compose up |

### API endpoints (25 REST + 3 WebSocket)
- Devices: GET /api/devices, POST /api/connect, /api/disconnect, /api/pair
- Info: GET /api/device/info, /api/screenshot
- Shell: POST /api/shell
- Packages: GET/POST /api/packages/*
- Files: GET/POST/DELETE /api/files/*
- Processes: GET /api/processes, POST /api/processes/:pid/kill
- Input: POST /api/input/{key,text,tap,swipe}
- Settings: GET/PUT /api/settings/{global,secure,system}
- Discovery: GET /api/discover (mDNS + TCP scan port 5555)
- Power: POST /api/reboot, /api/screen/toggle
- WebSocket: /ws/terminal, /ws/logcat, /ws/screenstream

### Frontend features
- Auto-detection transport (Extension > Agent > WebUSB)
- Page Connexion par defaut, nav cachee si pas d'appareil connecte
- Scan reseau auto au chargement de la page Connexion
- Terminal xterm.js avec PTY (via `script -qfc`)
- Screen: stream screenshots + tap/swipe + mode plein ecran avec barre flottante
- Logcat: filtres par niveau (V/D/I/W/E/F) + recherche
- Responsive (sidebar collapse en bottom navbar mobile)

### Origine du style
Inspire du projet RemoteADB (`/mnt/windows/temp/remoteadb/`) - dark theme avec variables CSS --bg-primary:#0f0f0f, --accent:#1976d2, etc.
