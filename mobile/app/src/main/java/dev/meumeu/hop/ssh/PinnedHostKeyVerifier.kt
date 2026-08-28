package dev.meumeu.hop.ssh

import android.util.Base64
import net.schmizz.sshj.common.Buffer
import net.schmizz.sshj.transport.verification.HostKeyVerifier
import java.security.PublicKey

/**
 * Verifie que le serveur presente EXACTEMENT la cle hote attendue.
 *
 * La cle est fournie au format `authorized_keys` ("ssh-ed25519 AAAA...").
 * On compare l'encodage binaire de la cle presentee a celui de la cle
 * epinglee — pas une empreinte tronquee, pas un prefixe.
 *
 * Sans cette verification, un tiers detenant le secret du tunnel Cloudflare
 * peut se faire passer pour la machine et recuperer la passphrase LUKS que
 * l'utilisateur tape en croyant parler a son serveur.
 */
class PinnedHostKeyVerifier(expectedAuthorizedKey: String) : HostKeyVerifier {

    private val expected: ByteArray? = parse(expectedAuthorizedKey)

    override fun verify(hostname: String?, port: Int, key: PublicKey?): Boolean {
        if (expected == null || key == null) return false
        val presented = Buffer.PlainBuffer().putPublicKey(key).compactData
        // Comparaison en temps constant : evite de fuiter par le temps de
        // reponse quelle portion de la cle correspond.
        if (presented.size != expected.size) return false
        var diff = 0
        for (i in presented.indices) {
            diff = diff or (presented[i].toInt() xor expected[i].toInt())
        }
        return diff == 0
    }

    override fun findExistingAlgorithms(hostname: String?, port: Int): List<String> = emptyList()

    private fun parse(authorizedKey: String): ByteArray? {
        val parts = authorizedKey.trim().split(Regex("\\s+"))
        // "<type> <base64>" — un eventuel commentaire final est ignore.
        val b64 = parts.getOrNull(1) ?: return null
        return try {
            Base64.decode(b64, Base64.DEFAULT)
        } catch (_: Exception) {
            null
        }
    }
}
