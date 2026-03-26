---
name: Feedback Testing VPN
description: Ne jamais faire de test full tunnel VPN depuis le PC principal, ne pas casser la connexion de l'utilisateur
type: feedback
---

- Ne JAMAIS faire de test VPN full tunnel (AllowedIPs = 0.0.0.0/0) depuis le PC principal
  **Why:** Un test full tunnel redirige TOUT le trafic du PC, y compris le SSH vers le bastion, ce qui coupe la connexion et bloque l'utilisateur. L'utilisateur a dû annuler manuellement.
  **How to apply:** Tester les tunnels VPN depuis un autre appareil, un namespace réseau isolé, ou un container Docker. Ne jamais affecter la connectivité du PC depuis lequel on travaille.

- Quand on fait des opérations réseau, toujours vérifier qu'on ne va pas couper l'accès SSH au bastion
  **Why:** Même raison - si on perd le SSH on ne peut plus rien faire à distance
  **How to apply:** Avant toute modif réseau/iptables/routage, vérifier que le chemin SSH reste intact

- Ne JAMAIS déployer de services réseau (WireGuard, containers VPN, bridges, iptables) directement sur le PC principal
  **Why:** wg-easy déployé sur le PC a niqué le réseau de l'utilisateur. Les services réseau peuvent capturer/rediriger le trafic et couper la connexion.
  **How to apply:** Toujours proposer de déployer sur le bastion ou une VM, JAMAIS sur le PC. Si le PC est la cible, demander confirmation explicite et ne faire qu'une étape à la fois avec vérification réseau entre chaque.
