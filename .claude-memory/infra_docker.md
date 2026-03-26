---
name: Docker Infrastructure
description: Stack Docker locale - containers AI, dashboard, services, reseau meumeu, modeles partages
type: reference
---

## Reseau Docker
- Reseau custom `meumeu` pour resolution DNS entre containers (le bridge par defaut ne supporte pas le DNS par nom)
- Tous les containers sont connectes au reseau `meumeu`

## Containers actifs
- ollama (GPU, port 11434)
- ollama-cpu (CPU, port 11435, phi3:mini)
- ollama-router (port 11436, lie ollama+ollama-cpu+claude-gateway)
- dashboard (port 80, monte docker.sock, image dashboard-app rebuild local)
- libretranslate (port 5000)
- portainer (port 9443, HTTPS)
- comfyui (GPU, port 8188, ai-dock image)
- fooocus (GPU, port 7865)
- automatic1111 (GPU, port 7860, ai-dock image)

## Services natifs (plus en Docker)
- openclaw (/usr/bin/openclaw) - installe en natif, config dans ~/.openclaw/openclaw.json

## GPU exclusif
- Un seul container GPU a la fois (le dashboard arrete les autres via stop_other_gpu)
- GPU_CONTAINERS = comfyui, fooocus, automatic1111, ollama

## Modeles IA partages
- Dossier host: /home/freelux/ai-models/ (checkpoints, loras, vae, embeddings, controlnet, clip, clip_vision, upscale_models)
- Monte en /shared-models:ro dans comfyui, automatic1111, fooocus
- ComfyUI: extra_model_paths.yaml pointe vers /shared-models/
- A1111: script /opt/link-shared-models.sh cree des symlinks (a relancer apres ajout de modeles)
- Fooocus: symlinks dans /content/data/models/

## Auth conteneurs ai-dock (comfyui, automatic1111)
- Caddy interne avec auth par cookie/token
- WEB_ENABLE_AUTH=false desactive l'auth (sinon redirige vers localhost:1111/login inaccessible)
- Cette config ne persiste pas au redemarrage: relancer `env-store WEB_ENABLE_AUTH && supervisorctl restart caddy`

## Credentials
- OVH API: ~/Documents/token-ovh.txt (AK: 470009e489ca0299)
- Cloudflare: aurelien.meunier@ik.me, Account ID: 45da3b1f136bc528046b4c9e8af11e4e
- CivitAI API token: e77324b713e9cfd9341f3edff34558d4
