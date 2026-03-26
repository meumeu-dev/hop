---
name: Dashboard App
description: Dashboard Docker local avec monitoring systeme, gestion GPU containers, traduction, prompt builder
type: project
---

Fichier: ~/meumeudev/apps/dashboard-app/
Docker: dashboard, port 80, reseau meumeu, monte docker.sock
Image: dashboard-app (build local via Dockerfile)
Auth: Basic (freelux/tgboulet), bypass reseau local

Fonctionnalites:
- Gestion containers Docker (start/stop/restart)
- GPU exclusif: un seul container GPU a la fois (comfyui, fooocus, automatic1111, ollama)
- Monitoring systeme: CPU, RAM, GPU (nvidia-smi), Disk, Network, Swap
- Router AI usage stats (cloud/claude/local requests)
- Prompt Builder (collapsible, section AI)
- Traduction FR/EN via LibreTranslate (http://libretranslate:5000 via reseau meumeu)
- Prompt enhancer via phi3:mini (http://ollama-cpu:11434 via reseau meumeu)

Containers retires du dashboard: webadb (supprime le 13/03/2026)

**How to apply:** Pour toute modif de app.py ou index.html: rebuild image (`docker build -t dashboard-app .`) puis recreer le container. Le simple restart ne suffit PAS car le fichier est copie dans l'image au build.
