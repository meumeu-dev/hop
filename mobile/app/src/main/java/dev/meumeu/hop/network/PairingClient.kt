package dev.meumeu.hop.network

import com.google.gson.Gson
import com.google.gson.annotations.SerializedName
import android.util.Log
import dev.meumeu.hop.crypto.HopCrypto
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.util.concurrent.TimeUnit

data class PairData(
    val hostname: String,
    val ip: String? = null,
    val ips: List<String>? = null,
    val user: String,
    @SerializedName("public_key") val publicKey: String,
    @SerializedName("host_key") val hostKey: String? = null,
    @SerializedName("cf_domain") val cfDomain: String? = null,
    @SerializedName("cf_env") val cfEnv: String? = null,
    val version: String? = null
)

// Since v3 the 8-char pairing code is the only session identifier.
data class PairSession(val code: String)

class PairingClient(private val workerUrl: String = DEFAULT_WORKER_URL) {

    companion object {
        const val DEFAULT_WORKER_URL = "https://hop-pair.meumeudev.workers.dev"
    }

    private val client = OkHttpClient.Builder()
        .connectTimeout(30, TimeUnit.SECONDS)
        .readTimeout(30, TimeUnit.SECONDS)
        .build()

    private val gson = Gson()
    private val jsonType = "application/json".toMediaType()

    fun publishPairData(code: String, data: PairData): PairSession {
        val jsonData = gson.toJson(data).toByteArray()
        val encrypted = HopCrypto.encrypt(jsonData, code)

        val body = gson.toJson(mapOf("code" to code, "data" to encrypted)).toRequestBody(jsonType)
        val request = Request.Builder()
            .url("$workerUrl/pair")
            .post(body)
            .build()

        val response = client.newCall(request).execute()
        when (response.code) {
            409 -> throw IllegalStateException("Code deja utilise, retente")
        }
        require(response.isSuccessful) { "Erreur serveur: HTTP ${response.code}" }
        return PairSession(code = code)
    }

    fun fetchPairData(code: String): PairData {
        Log.d("HOP-PAIR", "fetchPairData code=$code")
        val request = Request.Builder()
            .url("$workerUrl/pair/${code.trim()}")
            .get()
            .build()

        val response = client.newCall(request).execute()
        Log.d("HOP-PAIR", "fetchPairData response: ${response.code}")
        if (response.code == 404) throw IllegalStateException("Pairing non trouve ou expire (2min TTL)")
        require(response.isSuccessful) { "Erreur serveur: HTTP ${response.code}" }

        val bodyStr = response.body?.string()
        if (bodyStr.isNullOrBlank()) throw IllegalStateException("Reponse vide du serveur")

        val result = gson.fromJson(bodyStr, Map::class.java)
        val encrypted = result["data"] as? String
        if (encrypted.isNullOrBlank()) {
            throw IllegalStateException("Donnees de pairing manquantes dans la reponse")
        }

        val decrypted = HopCrypto.decrypt(encrypted, code)
        return gson.fromJson(String(decrypted), PairData::class.java)
    }

    fun sendResponse(session: PairSession, data: PairData) {
        val jsonData = gson.toJson(data).toByteArray()
        val encrypted = HopCrypto.encrypt(jsonData, session.code)

        val body = gson.toJson(mapOf("data" to encrypted)).toRequestBody(jsonType)
        val request = Request.Builder()
            .url("$workerUrl/pair/${session.code}/response")
            .post(body)
            .build()

        val response = client.newCall(request).execute()
        when (response.code) {
            429 -> throw IllegalStateException("Trop de reponses postees (DoS ?)")
        }
        require(response.isSuccessful) { "Erreur serveur: HTTP ${response.code}" }
    }

    // Poll up to 5 response slots, first that decrypts wins. Fake responses
    // from an attacker who doesn't know the code are silently skipped.
    fun waitForResponse(session: PairSession, timeoutMs: Long = 120_000): PairData {
        val deadline = System.currentTimeMillis() + timeoutMs
        while (System.currentTimeMillis() < deadline) {
            for (idx in 0 until 5) {
                val request = Request.Builder()
                    .url("$workerUrl/pair/${session.code}/response?idx=$idx")
                    .get()
                    .build()
                val response = client.newCall(request).execute()
                if (response.code == 404) { response.close(); continue }
                val bodyStr = response.body?.string().orEmpty()
                response.close()
                try {
                    val result = gson.fromJson(bodyStr, Map::class.java)
                    val encrypted = result["data"] as? String ?: continue
                    val decrypted = HopCrypto.decrypt(encrypted, session.code)
                    return gson.fromJson(String(decrypted), PairData::class.java)
                } catch (_: Exception) { /* wrong code, keep trying */ }
            }
            Thread.sleep(2000)
        }
        throw IllegalStateException("Timeout: pas de reponse recue")
    }

    fun cleanup(session: PairSession) {
        try {
            val request = Request.Builder()
                .url("$workerUrl/pair/${session.code}")
                .delete()
                .build()
            client.newCall(request).execute().close()
        } catch (_: Exception) {}
    }
}
