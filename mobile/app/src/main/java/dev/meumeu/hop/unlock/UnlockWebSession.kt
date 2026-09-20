package dev.meumeu.hop.unlock

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import java.io.IOException
import java.security.KeyFactory
import java.security.PublicKey
import java.security.spec.MGF1ParameterSpec
import java.security.spec.X509EncodedKeySpec
import java.util.Base64
import java.util.concurrent.TimeUnit
import javax.crypto.Cipher
import javax.crypto.spec.OAEPParameterSpec
import javax.crypto.spec.PSource

/**
 * Deverrouillage via le WEB d'unlock, en deux modes :
 *
 *  - Mode DIRECT (legacy, avant 2026-09-20) : on parle au serveur web de la
 *    machine (hostname = ingress du tunnel), auth par service token CF Access.
 *
 *  - Mode WORKER (mode courant) : tout passe par le Worker hop-pair, qui
 *    proxy /pubkey et /unlock vers la bonne machine (registry KV). Auth par
 *    le Bearer token du COMPTE hop -> l'utilisateur n'a plus rien à
 *    configurer (ni hostname, ni service token). L'app appelle
 *    `https://<worker>/pubkey?machine=<id>` et `/unlock?machine=<id>`.
 *
 * Protocole commun (page web, mode direct, mode worker) :
 *  1. GET  /pubkey                    -> { alg: "RSA-OAEP-256", pubkey: <DER base64> }
 *  2. chiffrer la passphrase en RSA-OAEP (SHA-256, MGF1=SHA-256) — identique
 *     a WebCrypto et au dechiffrement Go cote serveur
 *  3. POST /unlock { blob: <base64> } -> { ok: true }
 *
 * Aucune passphrase ne transite en clair sur le reseau : elle n'existe que
 * dans la memoire du telephone (chiffree ici) et est dechiffree en RAM par la
 * machine. Cloudflare ne voit qu'un blob chiffre.
 */
class UnlockWebSession private constructor(
    private val baseUrl: String,
    private val machineSuffix: String,
    private val authHeaders: Map<String, String>,
) {
    private val client: OkHttpClient = OkHttpClient.Builder()
        .connectTimeout(15, TimeUnit.SECONDS)
        .readTimeout(60, TimeUnit.SECONDS) // la machine peut mettre du temps avant de repondre
        .writeTimeout(30, TimeUnit.SECONDS)
        .build()

    private val json = "application/json; charset=utf-8".toMediaType()

    data class Result(
        val ok: Boolean,
        val msg: String,
    )

    /** Mode DIRECT : contacte le serveur web de la machine via son ingress. */
    companion object {
        /** Mode DIRECT : contacte le serveur web de la machine via son ingress. */
        fun direct(
            hostname: String,
            serviceTokenId: String,
            serviceTokenSecret: String,
        ) = UnlockWebSession(
            baseUrl = "https://$hostname",
            machineSuffix = "",
            authHeaders = mapOf(
                "Cf-Access-Client-Id" to serviceTokenId,
                "Cf-Access-Client-Secret" to serviceTokenSecret,
            )
        )

        /** Mode WORKER : l'unlock passe par hop-pair, auth = compte hop. */
        fun viaWorker(
            workerUrl: String,
            accountToken: String,
            machineId: String,
        ) = UnlockWebSession(
            baseUrl = workerUrl,
            machineSuffix = "?machine=$machineId",
            authHeaders = mapOf("Authorization" to "Bearer $accountToken")
        )
    }

    /**
     * Effectue un cycle complet d'unlock. Retourne [Result] ou leve une
     * exception ([IOException] reseau, [SecurityException] cle invalide...).
     */
    suspend fun unlock(passphrase: String): Result = withContext(Dispatchers.IO) {
        if (passphrase.isEmpty()) throw IllegalArgumentException("passphrase vide")

        val pubkey: PublicKey = run {
            val req = Request.Builder()
                .url("$baseUrl/pubkey$machineSuffix")
                .apply { authHeaders.forEach { (k, v) -> header(k, v) } }
                .build()
            client.newCall(req).execute().use { res ->
                if (!res.isSuccessful) throw IOException("pubkey: HTTP ${res.code}")
                val body = res.body?.string() ?: throw IOException("pubkey: reponse vide")
                try {
                    val json = JSONObject(body)
                    if (json.optString("alg") != "RSA-OAEP-256") {
                        throw IOException("alg non supporte: ${json.optString("alg")}")
                    }
                    val der = Base64.getDecoder().decode(json.getString("pubkey"))
                    KeyFactory.getInstance("RSA").generatePublic(X509EncodedKeySpec(der))
                } catch (e: Exception) {
                    if (e is IOException) throw e
                    throw IOException("pubkey illisible", e)
                }
            }
        }

        // RSA-OAEP avec SHA-256 ET MGF1=SHA-256 : les deux digests DOIVENT
        // etre en SHA-256 pour etre compatibles avec WebCrypto (page web) et
        // le dechiffrement Go (rsa.DecryptOAEP(sha256.New(), ...)).
        val cipher = Cipher.getInstance("RSA/ECB/OAEPPadding")
        cipher.init(
            Cipher.ENCRYPT_MODE,
            pubkey,
            OAEPParameterSpec("SHA-256", "MGF1", MGF1ParameterSpec.SHA256, PSource.PSpecified.DEFAULT),
        )
        val blob = cipher.doFinal(passphrase.toByteArray(Charsets.UTF_8))

        val payload = JSONObject().put("blob", Base64.getEncoder().encodeToString(blob)).toString()
        val send = Request.Builder()
            .url("$baseUrl/unlock$machineSuffix")
            .apply { authHeaders.forEach { (k, v) -> header(k, v) } }
            .post(payload.toRequestBody(json))
            .build()
        client.newCall(send).execute().use { res ->
            val text = res.body?.string() ?: ""
            val j = try {
                JSONObject(text)
            } catch (e: Exception) {
                throw IOException("reponse illisible (HTTP ${res.code})")
            }
            val msg = (if (j.optBoolean("ok")) (j.optString("msg").ifEmpty { "Disque déverrouillé" })
                       else (j.optString("error").ifEmpty { "Échec (HTTP ${res.code})" }))
            Result(ok = j.optBoolean("ok"), msg = msg)
        }
    }
}