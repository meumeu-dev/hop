---
name: project_planneria
description: PlannerIA - Assistant de planification securite privee (Securitas) avec IA multi-LLM, Docker, FastAPI+React
type: project
---

# PlannerIA

**Chemin:** `~/meumeudev/apps/plannerIA/`
**URL:** https://planner.meumeu.dev (via Cloudflare Tunnel)
**Stack:** FastAPI (async) + React 18 + Vite + Tailwind CSS + PostgreSQL 16 + Docker Compose + Nginx

## Architecture
- **Backend:** FastAPI, SQLAlchemy async (asyncpg), JWT auth (jose), bcrypt, litellm, aiosmtplib
- **Frontend:** React 18, Vite, Tailwind CSS, theme dark slate, UI 100% francais
- **Infra:** Docker Compose (4 services: db, backend, frontend, nginx), Cloudflare Tunnel
- **DB:** PostgreSQL 16 Alpine, volume `planneria_postgres_data`

## Hierarchie metier
Company → COL (Centre Operationnel Local) → Team → Member
- Convention Collective IDCC 1351 (securite privee)
- 31 regles metier Code du travail + CC injectees
- Membres avec COL principal/secondaire (is_primary dans member_col)

## Fonctionnalites implementees
- Auth JWT (access 1h + refresh 7j) avec rate limiting (slowapi)
- Registration admin-only
- Gestion entreprises/COLs/equipes/membres avec ownership strict (IDOR fix)
- Planning avec taches assignees
- Chat IA multi-LLM (Ollama, Claude, Gemini, OpenAI, custom) via litellm + streaming SSE
- Regles metier avec extraction/validation IA
- Envoi planning par email (aiosmtplib, config SMTP admin-only)
- Dashboard stats

## Securite (pentest mars 2026)
- SECRET_KEY crypto-random (openssl rand -hex 32), refuse demarrage si placeholder
- FERNET_KEY pour chiffrement API keys LLM (cryptography.fernet)
- Port DB 5432 non expose, mot de passe DB fort
- CORS restreint aux origines configurees (CORS_ORIGINS dans .env)
- Rate limiting login (10/min), refresh (30/min)
- IDOR corrige: Member a company_id, toutes les queries scoped par ownership
- Validation inputs: password complexite, EmailStr, max_length partout, no newlines SMTP
- Refresh tokens avec rotation + blacklist + endpoint logout
- Container backend non-root (USER appuser), pas de --reload en prod
- Headers securite: X-Frame-Options DENY, X-Content-Type-Options nosniff, Referrer-Policy, Permissions-Policy
- Docs API desactivees (docs_url=None, redoc_url=None)
- Error sanitization (pas de str(e) dans les reponses)

## DB actuelle
- Users: admin (id:1), freelux (id:2, admin), yann (id:3)
- Companies: Ma Societe (id:1, owner:admin), Securitas (id:2, owner:freelux)
- COLs Securitas: Poitiers (id:2), Niort (id:3), La Rochelle (id:4)
- 6 LLM configs par user (3 Claude via proxy, 3 Ollama local)

## TODO futur (demande user)
- Module client/site: gestion sites clients avec habilitations/formations/droits legaux des membres
- Optimisation tournees (VRP) pour agents de securite mobile
- Chiffrement donnees au repos
- Migration vers Redis pour blacklist tokens (actuellement in-memory)

**Why:** L'user developpe un outil de gestion planning pour entreprise de securite privee, avec emphase sur la conformite legale et la securite des donnees
**How to apply:** Toujours verifier ownership/IDOR sur les nouveaux endpoints, valider les inputs, pas de secrets en clair
