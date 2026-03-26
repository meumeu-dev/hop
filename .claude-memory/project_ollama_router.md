---
name: Ollama Smart Router
description: Routeur intelligent LLM avec sticky model, commandes !, Claude/Cloudflare proxy - port 11436
type: project
---

Fichier: ~/meumeudev/ai/ollama-router/app.py
Docker: ollama-router, port 11436, lie a ollama, ollama-cpu, claude-gateway
Env vars: CF_ACCOUNT_ID, CF_API_EMAIL, CF_API_KEY

Fonctionnalites:
- Routage auto via phi3:mini (CPU) vers modeles GPU
- Commandes ! : !help, !auto, !cloud, !claude, !sonnet, !opus, !haiku, !reasoning, !creative, !code, !chat, !uncensored, !big, !quick
- **Sticky model**: le modele choisi avec ! reste actif dans la conversation, !auto pour reset
- Proxy Claude via claude-code-gateway (port 8080)
- Proxy Cloudflare Workers AI (Llama 3.1 8B)
- Usage tracking (/api/usage) avec reset quotidien
- Prefixes: *[tag -> model]* pour override explicite, _tag -> model_ pour auto-route

**Why:** Permet d'utiliser tous les modeles via une seule interface Open WebUI
**How to apply:** Toute modif du routeur necessite rebuild docker + restart container
