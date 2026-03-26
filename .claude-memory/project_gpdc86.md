---
name: GPDC86 + DodgeBot
description: Site GPDC86 (gpdc86.meumeu.dev) + DodgeBot multi-club (dodgebot.fr) - CF Workers, D1, Workers AI, deux comptes CF separes
type: project
---

## GPDC86 - Site du club
- **Path:** /home/freelux/meumeudev/hosted/gpdc86
- **URL:** gpdc86.meumeu.dev (custom domain CF Workers)
- **Compte CF:** aurelien.meunier@ik.me (token: /home/freelux/meumeudev/#auth/meumeu.dev/token-cf.txt)
- **Stack:** Hono + TypeScript + CF Workers + D1
- **Couleurs:** Rouge/noir
- **Fonctionnalites:** Pages publiques (accueil, club, equipe, galerie, actus, documents, FAQ, contact), admin panel, widget DodgeBot integre avec chat texte + vocal (STT/TTS via Web Speech API), selecteur vitesse lecture (x1/x1.5/x2), indicateur micro actif, infos modele LLM
- **FAQ:** iframe vers dodgebot.fr/faq (pas duplique, une seule source)
- **Instagram:** https://www.instagram.com/poitiersdodgeball/ (dans footer + bot info)
- **Proxy DodgeBot:** /api/dodgebot → dodgebot.fr/api/chat (auth via API key)
- **Email:** poitiersdc@gmail.com (obfusque JS anti-spam)
- **Telephone:** retire du site public (securite)
- **Adresse:** 3 champs (adresse_lieu, adresse_rue, adresse_cp_ville) pour affichage multi-lignes
- **Securite:** CSRF, rate limiting, security headers, Permissions-Policy microphone=(self)
- **Widget DodgeBot:** Bouton FAB avec logo robot DodgeBot (static/logos/dodgebot.png), iframe vers dodgebot.fr/gpdc86?embed
- **IMPORTANT:** L'iframe DodgeBot ne fonctionne que depuis gpdc86.meumeu.dev (CSP frame-ancestors), PAS depuis gpdc86.meumeudev.workers.dev

## DodgeBot - Assistant IA multi-club standalone
- **Path:** /home/freelux/meumeudev/hosted/dodgebot
- **URL:** dodgebot.fr (custom domain CF Workers) - migre depuis dodgebot.meumeu.dev le 2026-03-14
- **Domaine:** dodgebot.fr (achete OVH, NS pointes vers CF)
- **Compte CF:** contact@dodgebot.fr (token: /home/freelux/meumeudev/#auth/dodgebot.fr/token-cf.txt)
- **Account ID:** 2f9de09a8c8a6ab4c13fafc66a93a332
- **D1 ID:** 3ecc34b0-0408-42c3-8a7d-20663db77b80
- **Stack:** Hono + TypeScript + CF Workers + D1 + Workers AI + ASSETS binding (SPA)
- **Modele:** @cf/meta/llama-3.3-70b-instruct-fp8-fast
- **Architecture multi-club:**
  - Un fichier par club dans src/clubs/ (ex: gpdc86.ts)
  - Index central src/clubs/index.ts avec ClubProfile/ClubTheme interfaces
  - Route /:slug sert index.html via ASSETS.fetch() (SPA routing)
  - Client JS detecte le slug dans l'URL, charge le profil club via /api/club/:slug
  - Theme dynamique via CSS custom properties (couleurs, logo, bg)
  - Clubs peuvent choisir d'etre visibles ou non sur la homepage
  - Bot generique sur / avec connaissances generales dodgeball (~26 clubs FR)
- **Logo:** Robot mascotte custom (logo.png + favicon.png dans public/), genere via DALL-E, fond transparent
- **Fonctionnalites:** Chat UI (dark theme), voix (STT/TTS), sessions persistantes, auth (admin-only), API keys, MCP configs, FAQ page (/faq.html), regles PDF upload/download (D1 base64), liens PDF dans reponses bot quand il cite des articles, zone actualites sur page d'accueil, systeme de report/signalement de messages bot
- **Tables D1:** users, api_keys, chat_sessions, chat_messages, mcp_servers, login_attempts, rules_documents, news (actualites), reports (signalements)
- **Securite:** Anti-prompt-injection, output filtering, CORS restreint (dodgebot.fr + gpdc86.meumeu.dev), security headers, CSP frame-ancestors, HTTPS force, TLS 1.2 min, HSTS, security level high
- **System prompt:** dodgebot-rules.ts (regles FFDodgeball v3.2) + club info OU dodgebot-general.ts (connaissances generales)
- **Identite:** "DodgeBot - Ton assistant dodgeball"
- **www redirect:** www.dodgebot.fr → dodgebot.fr (301 via page rule)

## Infos club
- SIRET 939636569
- Halle des sports Claude Dasriaux, 55 rue de Poitiers, 86440 Migne-Auxances
- President: Guillaume Fournet, Coach: Vincent Renaud
- Entrainements: Jeudi 20h-22h
- Facebook: facebook.com/p/Grand-Poitiers-Dodgeball-61561918232099/
- Instagram: instagram.com/poitiersdodgeball/

## OVH
- Token API OVH: /home/freelux/meumeudev/#auth/token-ovh-14032026.txt
- Domaine dodgebot.fr enregistre chez OVH, NS delegues a CF

**Why:** Le club n'a pas de site web. DodgeBot separe sur son propre domaine/compte CF pour etre reutilisable par d'autres clubs.
**How to apply:** Deployer dodgebot avec CLOUDFLARE_API_KEY + CLOUDFLARE_EMAIL=contact@dodgebot.fr. Deployer gpdc86 avec le compte meumeu.dev. Ne pas remettre le telephone sur le site public.
