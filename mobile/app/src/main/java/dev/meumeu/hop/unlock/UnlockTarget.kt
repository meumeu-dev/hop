package dev.meumeu.hop.unlock

import java.util.UUID

/**
 * Une machine que l'utilisateur peut deverrouiller a distance. Tout est saisi
 * par l'utilisateur dans l'app — rien n'est code en dur, chacun vise ses
 * propres appareils.
 *
 * Le deverrouillage passe par le serveur WEB d'unlock de la machine (ingress
 * du tunnel) : l'app recupère la cle publique ephemere du serveur
 * ([/)pubkey), chiffre la passphrase en RSA-OAEP-256 dans le terminal, et
 * l'envoie en POST /unlock. Le serveur la dechiffre en RAM et la passe à
 * cryptsetup. Aucune cle SSH, aucun flux terminal : uniquement HTTP.
 *
 * Contient des secrets (token CF Access) : stocke chiffre via
 * [dev.meumeu.hop.HopConfig.saveUnlockTargets].
 */
data class UnlockTarget(
    val id: String = UUID.randomUUID().toString(),
    /** Nom court, sert aussi d'identifiant cote Worker pour l'etat d'attente. */
    val machineId: String,
    /** Hostname du tunnel Cloudflare vers le WEB d'unlock, ex: unlock-web-machin.exemple.com */
    val hostname: String,
    val serviceTokenId: String,
    val serviceTokenSecret: String,
) {
    /** Verifie que les champs obligatoires sont remplis et coherents. */
    fun validate(): String? = when {
        machineId.isBlank() -> "Le nom de la machine est requis"
        !machineId.matches(Regex("^[a-zA-Z0-9_-]{1,32}$")) ->
            "Nom invalide (lettres, chiffres, - et _ uniquement, 32 max)"
        hostname.isBlank() -> "Le hostname du tunnel est requis"
        hostname.contains("/") || hostname.contains(" ") ->
            "Hostname invalide (ex: unlock-web-machin.exemple.com)"
        serviceTokenId.isBlank() -> "Le Client ID du service token est requis"
        serviceTokenSecret.isBlank() -> "Le Client Secret du service token est requis"
        else -> null
    }
}