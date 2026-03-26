---
name: feedback_security
description: L'user exige securite maximale sur PlannerIA - chiffrement, aucune fuite, tout securise
type: feedback
---

"tout doit etre securise, chiffre, aucune fuite possible"

**Why:** L'app gere des donnees sensibles (plannings securite privee, donnees personnelles membres, habilitations). Le user est conscient des enjeux RGPD et securite.

**How to apply:**
- Toujours verifier IDOR/ownership sur chaque endpoint
- Chiffrer les donnees sensibles (API keys, credentials) avec Fernet
- Ne jamais exposer d'erreurs internes (str(e)) dans les reponses API
- Valider tous les inputs (max_length, regex, EmailStr)
- Rate limiting sur les endpoints sensibles
- Pas de secrets en clair dans le code ou les configs par defaut
