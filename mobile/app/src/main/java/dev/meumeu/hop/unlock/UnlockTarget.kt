package dev.meumeu.hop.unlock

import android.content.Context
import java.io.File
import java.util.UUID

/**
 * Une machine que l'utilisateur peut deverrouiller a distance. Tout est saisi
 * par l'utilisateur dans l'app — rien n'est code en dur, chacun vise ses
 * propres appareils.
 *
 * Contient des secrets (token CF Access + cle privee SSH) : stocke chiffre via
 * [dev.meumeu.hop.HopConfig.saveUnlockTargets].
 */
data class UnlockTarget(
    val id: String = UUID.randomUUID().toString(),
    /** Nom court, sert aussi d'identifiant cote Worker pour l'etat d'attente. */
    val machineId: String,
    /** Hostname du tunnel Cloudflare, ex: unlock-machin.exemple.com */
    val hostname: String,
    val serviceTokenId: String,
    val serviceTokenSecret: String,
    val privateKeyPem: String,
    /**
     * Cle hote SSH attendue de dropbear, format "ssh-ed25519 AAAA...".
     * Epinglee : sans elle, quiconque obtient le secret du tunnel peut se
     * faire passer pour la machine et capturer la passphrase saisie.
     * Vide = ancienne configuration, connexion non authentifiee.
     */
    val hostKey: String = "",
) {
    /**
     * Ecrit la cle privee dans le stockage prive de l'app (sshj a besoin d'un
     * fichier). Reecrite a chaque session pour rester synchronisee si
     * l'utilisateur modifie la cible.
     */
    fun writePrivateKeyFile(context: Context): File {
        val dir = File(context.filesDir, "unlock_keys").apply { mkdirs() }
        val f = File(dir, "key_$id")
        f.writeText(privateKeyPem.trim() + "\n")
        f.setReadable(false, false)
        f.setReadable(true, true)
        f.setWritable(false, false)
        f.setWritable(true, true)
        return f
    }

    fun deleteKeyFile(context: Context) {
        File(File(context.filesDir, "unlock_keys"), "key_$id").delete()
    }

    /** Verifie que les champs obligatoires sont remplis et coherents. */
    fun validate(): String? = when {
        machineId.isBlank() -> "Le nom de la machine est requis"
        !machineId.matches(Regex("^[a-zA-Z0-9_-]{1,32}$")) ->
            "Nom invalide (lettres, chiffres, - et _ uniquement, 32 max)"
        hostname.isBlank() -> "Le hostname du tunnel est requis"
        hostname.contains("/") || hostname.contains(" ") ->
            "Hostname invalide (ex: unlock-machin.exemple.com)"
        serviceTokenId.isBlank() -> "Le Client ID du service token est requis"
        serviceTokenSecret.isBlank() -> "Le Client Secret du service token est requis"
        !privateKeyPem.contains("PRIVATE KEY") ->
            "Clé privée SSH invalide (format OpenSSH attendu)"
        hostKey.isNotBlank() && !hostKey.trim().startsWith("ssh-") ->
            "Clé hôte invalide (format attendu : ssh-ed25519 AAAA...)"
        else -> null
    }
}
