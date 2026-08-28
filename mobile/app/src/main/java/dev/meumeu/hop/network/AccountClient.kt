package dev.meumeu.hop.network

import com.google.gson.Gson
import com.google.gson.annotations.SerializedName
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.bouncycastle.crypto.generators.Argon2BytesGenerator
import org.bouncycastle.crypto.params.Argon2Parameters
import java.net.URLEncoder
import java.security.SecureRandom
import java.util.Base64
import java.util.concurrent.TimeUnit
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

data class AccountSession(
    @SerializedName("account_id") val accountId: String,
    val username: String,
    val email: String,
    val token: String,
    @SerializedName("data_key") val dataKey: String // hex-encoded AES key
)

class AccountClient(private val workerUrl: String = PairingClient.DEFAULT_WORKER_URL) {

    private val client = OkHttpClient.Builder()
        .connectTimeout(30, TimeUnit.SECONDS)
        .readTimeout(30, TimeUnit.SECONDS)
        .build()

    private val gson = Gson()
    private val jsonType = "application/json".toMediaType()

    companion object {
        private const val GCM_NONCE_SIZE = 12
        private const val GCM_TAG_BITS = 128
        private const val KEY_SIZE = 32

        /** Derive auth hash with server-provided random salt (hex). Argon2id 3 iter, 64MB, 4 threads. */
        fun deriveAuthHash(password: String, saltHex: String): String {
            val salt = hexDecode(saltHex)
            return hexEncode(argon2id(password.toByteArray(Charsets.UTF_8), salt))
        }

        /** Derive data key with deterministic email-based salt. Never sent to server. */
        fun deriveDataKey(email: String, password: String): String {
            val salt = "hop-data:${email.lowercase()}".toByteArray(Charsets.UTF_8)
            return hexEncode(argon2id(password.toByteArray(Charsets.UTF_8), salt))
        }

        private fun argon2id(password: ByteArray, salt: ByteArray): ByteArray {
            val params = Argon2Parameters.Builder(Argon2Parameters.ARGON2_id)
                .withSalt(salt)
                .withIterations(3)
                .withMemoryAsKB(64 * 1024)
                .withParallelism(4)
                .build()

            val gen = Argon2BytesGenerator()
            gen.init(params)

            val key = ByteArray(KEY_SIZE)
            gen.generateBytes(password, key)
            return key
        }

        fun encryptData(data: ByteArray, hexKey: String): String {
            val key = hexDecode(hexKey)
            val nonce = ByteArray(GCM_NONCE_SIZE)
            SecureRandom().nextBytes(nonce)

            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.ENCRYPT_MODE, SecretKeySpec(key, "AES"), GCMParameterSpec(GCM_TAG_BITS, nonce))
            val ciphertext = cipher.doFinal(data)

            return Base64.getEncoder().encodeToString(nonce + ciphertext)
        }

        fun decryptData(encoded: String, hexKey: String): ByteArray {
            val key = hexDecode(hexKey)
            val raw = Base64.getDecoder().decode(encoded)
            require(raw.size > GCM_NONCE_SIZE) { "data too short" }

            val nonce = raw.sliceArray(0 until GCM_NONCE_SIZE)
            val ciphertext = raw.sliceArray(GCM_NONCE_SIZE until raw.size)

            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.DECRYPT_MODE, SecretKeySpec(key, "AES"), GCMParameterSpec(GCM_TAG_BITS, nonce))
            return cipher.doFinal(ciphertext)
        }

        fun hexEncode(bytes: ByteArray): String =
            bytes.joinToString("") { "%02x".format(it) }

        fun hexDecode(hex: String): ByteArray {
            require(hex.length % 2 == 0) { "invalid hex string" }
            return ByteArray(hex.length / 2) { i ->
                hex.substring(i * 2, i * 2 + 2).toInt(16).toByte()
            }
        }
    }

    /** Fetch the random salt for an email (step 1 of login) */
    private fun fetchSalt(email: String): String {
        val request = Request.Builder()
            .url("$workerUrl/auth/salt?email=${URLEncoder.encode(email, "UTF-8")}")
            .get()
            .build()

        val response = client.newCall(request).execute()
        val result = gson.fromJson(response.body!!.string(), Map::class.java)
        return result["salt"] as? String ?: throw Exception("Salt introuvable")
    }

    /** Register — client generates random salt, sends it with auth_hash */
    fun register(email: String, username: String, password: String): AccountSession {
        // Generate random salt
        val saltBytes = ByteArray(16)
        SecureRandom().nextBytes(saltBytes)
        val authSalt = hexEncode(saltBytes)

        val authHash = deriveAuthHash(password, authSalt)
        val dataKey = deriveDataKey(email, password)

        val body = gson.toJson(
            mapOf(
                "email" to email,
                "username" to username,
                "auth_hash" to authHash,
                "auth_salt" to authSalt
            )
        ).toRequestBody(jsonType)

        val request = Request.Builder()
            .url("$workerUrl/auth/register")
            .post(body)
            .build()

        val response = client.newCall(request).execute()
        val result = gson.fromJson(response.body!!.string(), Map::class.java)

        if (result["ok"] as? Boolean != true) {
            throw Exception(result["error"] as? String ?: "Erreur inconnue")
        }

        return AccountSession(
            accountId = result["account_id"] as String,
            username = result["username"] as String,
            email = email,
            token = result["token"] as String,
            dataKey = dataKey
        )
    }

    /** Login — 2-step: fetch salt then authenticate. Accepts email or username. */
    fun login(identifier: String, password: String): AccountSession {
        // Step 1: fetch salt (works with email or username)
        val salt = fetchSalt(identifier)

        // Step 2: compute hash with server's salt
        val authHash = deriveAuthHash(password, salt)

        val body = gson.toJson(
            mapOf("email" to identifier, "auth_hash" to authHash)
        ).toRequestBody(jsonType)

        val request = Request.Builder()
            .url("$workerUrl/auth/login")
            .post(body)
            .build()

        val response = client.newCall(request).execute()
        val result = gson.fromJson(response.body!!.string(), Map::class.java)

        if (result["ok"] as? Boolean != true) {
            throw Exception(result["error"] as? String ?: "Erreur inconnue")
        }

        // Use real email from server response for data key derivation (not the username input)
        val realEmail = result["email"] as? String ?: identifier
        val dataKey = deriveDataKey(realEmail, password)

        return AccountSession(
            accountId = result["account_id"] as String,
            username = result["username"] as String,
            email = realEmail,
            token = result["token"] as String,
            dataKey = dataKey
        )
    }

    fun logout(token: String) {
        try {
            val request = Request.Builder()
                .url("$workerUrl/auth/logout")
                .post("".toRequestBody(jsonType))
                .header("Authorization", "Bearer $token")
                .build()
            client.newCall(request).execute().close()
        } catch (_: Exception) {}
    }

    fun pushMachines(token: String, encryptedData: String) {
        val body = gson.toJson(mapOf("data" to encryptedData)).toRequestBody(jsonType)

        val request = Request.Builder()
            .url("$workerUrl/account/machines")
            .put(body)
            .header("Authorization", "Bearer $token")
            .build()

        val response = client.newCall(request).execute()
        when (response.code) {
            401 -> throw Exception("Session expiree, reconnectez-vous")
            200 -> { /* ok */ }
            else -> throw Exception("Erreur serveur: HTTP ${response.code}")
        }
    }

    /**
     * Envoie la config des machines a deverrouiller, deja chiffree cote client
     * (le Worker ne stocke qu'un blob opaque, il ne peut pas la lire).
     */
    fun pushUnlockConfig(token: String, encryptedData: String) {
        val body = gson.toJson(mapOf("data" to encryptedData)).toRequestBody(jsonType)
        val request = Request.Builder()
            .url("$workerUrl/account/unlock")
            .put(body)
            .header("Authorization", "Bearer $token")
            .build()
        val response = client.newCall(request).execute()
        when (response.code) {
            401 -> throw Exception("Session expiree, reconnectez-vous")
            200 -> { /* ok */ }
            else -> throw Exception("Erreur serveur: HTTP ${response.code}")
        }
    }

    /** Recupere le blob chiffre des machines a deverrouiller ("" si aucun). */
    fun pullUnlockConfig(token: String): String {
        val request = Request.Builder()
            .url("$workerUrl/account/unlock")
            .get()
            .header("Authorization", "Bearer $token")
            .build()
        val response = client.newCall(request).execute()
        if (response.code == 401) throw Exception("Session expiree, reconnectez-vous")
        val result = gson.fromJson(response.body!!.string(), Map::class.java)
        return result["data"] as? String ?: ""
    }

    /** Supprime la config synchronisee du cloud. */
    fun deleteUnlockConfig(token: String) {
        val request = Request.Builder()
            .url("$workerUrl/account/unlock")
            .delete()
            .header("Authorization", "Bearer $token")
            .build()
        client.newCall(request).execute().close()
    }

    fun pullMachines(token: String): String {
        val request = Request.Builder()
            .url("$workerUrl/account/machines")
            .get()
            .header("Authorization", "Bearer $token")
            .build()

        val response = client.newCall(request).execute()
        if (response.code == 401) throw Exception("Session expiree, reconnectez-vous")

        val result = gson.fromJson(response.body!!.string(), Map::class.java)
        return result["machines"] as? String ?: ""
    }
}
