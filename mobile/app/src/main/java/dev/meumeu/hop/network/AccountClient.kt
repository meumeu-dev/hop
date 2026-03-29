package dev.meumeu.hop.network

import com.google.gson.Gson
import com.google.gson.annotations.SerializedName
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.bouncycastle.crypto.generators.Argon2BytesGenerator
import org.bouncycastle.crypto.params.Argon2Parameters
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

        /**
         * Derive auth hash: Argon2id with salt "hop-auth:<email>", 3 iter, 64MB, 1 thread, 32 bytes.
         * Result is hex-encoded and sent to server for authentication.
         */
        fun deriveAuthHash(email: String, password: String): String {
            val salt = "hop-auth:${email.lowercase()}".toByteArray(Charsets.UTF_8)
            return hexEncode(argon2id(password.toByteArray(Charsets.UTF_8), salt))
        }

        /**
         * Derive data key: Argon2id with salt "hop-data:<email>", 3 iter, 64MB, 1 thread, 32 bytes.
         * Result is hex-encoded. Never sent to server — used to encrypt machine data client-side.
         */
        fun deriveDataKey(email: String, password: String): String {
            val salt = "hop-data:${email.lowercase()}".toByteArray(Charsets.UTF_8)
            return hexEncode(argon2id(password.toByteArray(Charsets.UTF_8), salt))
        }

        private fun argon2id(password: ByteArray, salt: ByteArray): ByteArray {
            val params = Argon2Parameters.Builder(Argon2Parameters.ARGON2_id)
                .withSalt(salt)
                .withIterations(3)
                .withMemoryAsKB(64 * 1024)
                .withParallelism(1)
                .build()

            val gen = Argon2BytesGenerator()
            gen.init(params)

            val key = ByteArray(KEY_SIZE)
            gen.generateBytes(password, key)
            return key
        }

        /**
         * Encrypt data with AES-256-GCM. Output: base64(nonce || ciphertext+tag).
         * Matches Go's EncryptData: gcm.Seal(nonce, nonce, data, nil) then base64.
         */
        fun encryptData(data: ByteArray, hexKey: String): String {
            val key = hexDecode(hexKey)
            val nonce = ByteArray(GCM_NONCE_SIZE)
            SecureRandom().nextBytes(nonce)

            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(
                Cipher.ENCRYPT_MODE,
                SecretKeySpec(key, "AES"),
                GCMParameterSpec(GCM_TAG_BITS, nonce)
            )
            val ciphertext = cipher.doFinal(data)

            // Go format: nonce || ciphertext+tag (Java GCM appends tag to ciphertext already)
            val result = nonce + ciphertext
            return Base64.getEncoder().encodeToString(result)
        }

        /**
         * Decrypt data with AES-256-GCM. Input: base64(nonce || ciphertext+tag).
         * Matches Go's DecryptData.
         */
        fun decryptData(encoded: String, hexKey: String): ByteArray {
            val key = hexDecode(hexKey)
            val raw = Base64.getDecoder().decode(encoded)
            require(raw.size > GCM_NONCE_SIZE) { "data too short" }

            val nonce = raw.sliceArray(0 until GCM_NONCE_SIZE)
            val ciphertext = raw.sliceArray(GCM_NONCE_SIZE until raw.size)

            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(
                Cipher.DECRYPT_MODE,
                SecretKeySpec(key, "AES"),
                GCMParameterSpec(GCM_TAG_BITS, nonce)
            )
            return cipher.doFinal(ciphertext)
        }

        private fun hexEncode(bytes: ByteArray): String =
            bytes.joinToString("") { "%02x".format(it) }

        private fun hexDecode(hex: String): ByteArray {
            require(hex.length % 2 == 0) { "invalid hex string" }
            return ByteArray(hex.length / 2) { i ->
                hex.substring(i * 2, i * 2 + 2).toInt(16).toByte()
            }
        }
    }

    /**
     * Register a new account. Returns session with data key derived client-side.
     */
    fun register(email: String, username: String, password: String): AccountSession {
        val authHash = deriveAuthHash(email, password)
        val dataKey = deriveDataKey(email, password)

        val body = gson.toJson(
            mapOf(
                "email" to email,
                "username" to username,
                "auth_hash" to authHash
            )
        ).toRequestBody(jsonType)

        val request = Request.Builder()
            .url("$workerUrl/auth/register")
            .post(body)
            .build()

        val response = client.newCall(request).execute()
        val result = gson.fromJson(response.body!!.string(), Map::class.java)

        val ok = result["ok"] as? Boolean ?: false
        if (!ok) {
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

    /**
     * Login to an existing account. Returns session with data key derived client-side.
     */
    fun login(email: String, password: String): AccountSession {
        val authHash = deriveAuthHash(email, password)
        val dataKey = deriveDataKey(email, password)

        val body = gson.toJson(
            mapOf(
                "email" to email,
                "auth_hash" to authHash
            )
        ).toRequestBody(jsonType)

        val request = Request.Builder()
            .url("$workerUrl/auth/login")
            .post(body)
            .build()

        val response = client.newCall(request).execute()
        val result = gson.fromJson(response.body!!.string(), Map::class.java)

        val ok = result["ok"] as? Boolean ?: false
        if (!ok) {
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

    /**
     * Logout — invalidate the server-side session.
     */
    fun logout(token: String) {
        try {
            val request = Request.Builder()
                .url("$workerUrl/auth/logout")
                .post("".toRequestBody(jsonType))
                .header("Authorization", "Bearer $token")
                .build()
            client.newCall(request).execute().close()
        } catch (_: Exception) {
            // Best effort
        }
    }

    /**
     * Push encrypted machine data to the cloud.
     */
    fun pushMachines(token: String, encryptedData: String) {
        val body = gson.toJson(mapOf("data" to encryptedData)).toRequestBody(jsonType)

        val request = Request.Builder()
            .url("$workerUrl/account/machines")
            .put(body)
            .header("Content-Type", "application/json")
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
     * Pull encrypted machine data from the cloud.
     * Returns empty string if no data stored yet.
     */
    fun pullMachines(token: String): String {
        val request = Request.Builder()
            .url("$workerUrl/account/machines")
            .get()
            .header("Authorization", "Bearer $token")
            .build()

        val response = client.newCall(request).execute()
        if (response.code == 401) {
            throw Exception("Session expiree, reconnectez-vous")
        }

        val result = gson.fromJson(response.body!!.string(), Map::class.java)
        return result["machines"] as? String ?: ""
    }
}
