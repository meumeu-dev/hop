---
name: Feedback General
description: Retours utilisateur sur le comportement attendu - ne pas agir sans demander, pas de sur-ingenierie
type: feedback
---

- Ne pas installer de logiciels sans qu'on le demande explicitement
  **Why:** A installe testdisk/gddrescue/foremost sans qu'on demande, l'utilisateur a du faire desinstaller
  **How to apply:** Toujours demander avant d'installer quoi que ce soit

- Ne pas deployer/publier des projets sensibles en ligne sans confirmation explicite
  **Why:** A failli deployer AquaTree sur CF Pages, l'utilisateur a bloque car idee confidentielle
  **How to apply:** Pour les projets dans products/, toujours garder en local sauf demande explicite

- Ne pas modifier des services externes (supprimer zones DNS, etc.) sans confirmation
  **Why:** A lance un GET sur Cloudflare pour la zone meumeu.dev avant que l'utilisateur confirme, l'utilisateur a cru que ca supprimait
  **How to apply:** Toujours confirmer avant toute action sur des services externes (Cloudflare, OVH, etc.)

- Quand l'utilisateur dit "check" pour un processus long, afficher l'heure actuelle avec le statut

- Ne pas demander "tu veux que je fasse X ?" — faire directement
  **Why:** CLAUDE.md mis a jour par l'utilisateur: "Ne JAMAIS demander tu veux que je lance X"
  **How to apply:** Tester, corriger, enchainer. Pas de questions inutiles.

- Apres un pentest, corriger ET relancer un nouveau round automatiquement
  **Why:** L'utilisateur a du dire "tu corriges et tu repentest" plusieurs fois
  **How to apply:** Pentest → fix → repentest, boucler jusqu'a 0 critical/high
