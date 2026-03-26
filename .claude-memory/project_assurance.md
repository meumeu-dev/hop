---
name: Comparatif Assurances
description: Duel assurance Allianz vs Generali, comparatif en ligne sur meumeu.dev/duel, docs dans ~/Documents/assurance/
type: project
---

## Comparatif en ligne
- **URL:** meumeu.dev/duel (mot de passe: "duel")
- **Worker:** comparatif-assurance (~/Documents/assurance/worker-comparatif/)
- **KV namespace:** cfa83dc8efe843f9b4bb27199ebcbcdc (stocke les PDFs)
- **HTML local:** ~/Documents/assurance/comparatif.html

## Documents
- ~/Documents/assurance/allianz/ — Contrats et cotisations Allianz
- Anciens contrats renommes en _ancien (pas supprimes)
- Nommage: Allianz_{Type}_{Detail}_{Tarif}e.pdf

## Mise a jour du 16/03/2026
2 nouveaux avenants Allianz:
- MRH Chabournay: 584€ → 871€ (contrat 55283563, **piscine ajoutee**)
- PJ Protexia: 352€ inchange (contrat 56410176, passe de 3 a **2 biens** locatifs: Buxerolles 8B + Doussay)

Total Allianz: 3 741€ → 4 028€/an
Total Generali: 4 239€/an
Ecart: 498€ → 211€/an

**Why:** Avec la piscine ajoutee, la MRH Chabournay n'est plus qu'a 56€ d'ecart (vs 343€ avant). Passer tout chez Generali ne couterait plus que +18€/mois. Tesla, Polo, MRH et GAV sont quasi au meme prix.
**How to apply:** Deployer worker depuis ~/Documents/assurance/worker-comparatif/. PDFs uploades via `npx wrangler kv key put --remote`. Nouveaux PDFs dans allianz/: Allianz_MRH_Chabournay_7pieces_Piscine_871e.pdf, Allianz_PJ_VieQuotidienne-Plus_Protexia_352e.pdf
