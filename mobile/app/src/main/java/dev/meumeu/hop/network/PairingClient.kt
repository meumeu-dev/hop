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

data class PairSession(
    val pairId: String,
    val token: String,
    val code: String
)

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

        val body = gson.toJson(mapOf("data" to encrypted)).toRequestBody(jsonType)
        val request = Request.Builder()
            .url("$workerUrl/pair")
            .post(body)
            .build()

        val response = client.newCall(request).execute()
        require(response.isSuccessful) { "Erreur serveur: HTTP ${response.code}" }

        val result = gson.fromJson(response.body!!.string(), Map::class.java)
        return PairSession(
            pairId = result["pair_id"] as String,
            token = result["token"] as String,
            code = code
        )
    }

    fun fetchPairData(pairId: String, code: String): PairData {
        Log.d("HOP-PAIR", "fetchPairData pairId='$pairId' codeLen=${code.length}")
        val request = Request.Builder()
            .url("$workerUrl/pair/${pairId.trim()}")
            .get()
            .build()

        val response = client.newCall(request).execute()
        Log.d("HOP-PAIR", "fetchPairData response: ${response.code}")
        if (response.code == 404) throw IllegalStateException("Pairing non trouve ou expire (2min TTL)")
        require(response.isSuccessful) { "Erreur serveur: HTTP ${response.code}" }

        val bodyStr = response.body?.string()
        Log.d("HOP-PAIR", "fetchPairData body: ${bodyStr?.take(100)}")
        if (bodyStr.isNullOrBlank()) throw IllegalStateException("Reponse vide du serveur")

        val result = gson.fromJson(bodyStr, Map::class.java)
        val encrypted = result["data"] as? String
        if (encrypted.isNullOrBlank()) {
            Log.e("HOP-PAIR", "fetchPairData: 'data' field missing or null. Keys: ${result.keys}")
            throw IllegalStateException("Donnees de pairing manquantes dans la reponse")
        }

        Log.d("HOP-PAIR", "fetchPairData decrypting ${encrypted.length} chars...")
        val decrypted = HopCrypto.decrypt(encrypted, code)
        Log.d("HOP-PAIR", "fetchPairData decrypted: ${String(decrypted).take(100)}")
        return gson.fromJson(String(decrypted), PairData::class.java)
    }

    fun sendResponse(session: PairSession, data: PairData) {
        val jsonData = gson.toJson(data).toByteArray()
        val encrypted = HopCrypto.encrypt(jsonData, session.code)

        val body = gson.toJson(mapOf("data" to encrypted)).toRequestBody(jsonType)
        val request = Request.Builder()
            .url("$workerUrl/pair/${session.pairId}/response")
            .post(body)
            .header("X-Pair-Token", session.token)
            .build()

        val response = client.newCall(request).execute()
        when (response.code) {
            409 -> throw IllegalStateException("Une reponse a deja ete postee")
            401 -> throw IllegalStateException("Token invalide")
        }
        require(response.isSuccessful) { "Erreur serveur: HTTP ${response.code}" }
    }

    fun waitForResponse(session: PairSession, timeoutMs: Long = 120_000): PairData {
        val deadline = System.currentTimeMillis() + timeoutMs
        while (System.currentTimeMillis() < deadline) {
            val request = Request.Builder()
                .url("$workerUrl/pair/${session.pairId}/response")
                .get()
                .header("X-Pair-Token", session.token)
                .build()

            val response = client.newCall(request).execute()
            if (response.code == 404) {
                response.close()
                Thread.sleep(2000)
                continue
            }

            val result = gson.fromJson(response.body!!.string(), Map::class.java)
            val encrypted = result["data"] as String
            val decrypted = HopCrypto.decrypt(encrypted, session.code)
            return gson.fromJson(String(decrypted), PairData::class.java)
        }
        throw IllegalStateException("Timeout: pas de reponse recue")
    }

    fun cleanup(session: PairSession) {
        try {
            val request = Request.Builder()
                .url("$workerUrl/pair/${session.pairId}")
                .delete()
                .header("X-Pair-Token", session.token)
                .build()
            client.newCall(request).execute().close()
        } catch (_: Exception) {}
    }
}
