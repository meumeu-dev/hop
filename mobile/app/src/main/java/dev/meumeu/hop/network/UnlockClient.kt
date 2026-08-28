package dev.meumeu.hop.network

import com.google.gson.Gson
import com.google.gson.JsonParser
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody

private val JSON = "application/json".toMediaType()

/** Etat d'attente d'unlock d'une machine (voir plan resilient-napping-stroustrup.md). */
data class UnlockStatus(val pending: Boolean, val since: Long?)

/**
 * Appels Worker pour l'unlock LUKS a la demande. Pas de push : l'app
 * interroge le statut quand l'utilisateur l'ouvre.
 */
class UnlockClient(private val workerUrl: String = PairingClient.DEFAULT_WORKER_URL) {
    private val client = OkHttpClient()
    private val gson = Gson()

    /** Est-ce que cette machine attend un deverrouillage ? */
    fun status(accountToken: String, machineId: String): UnlockStatus {
        val req = Request.Builder()
            .url("$workerUrl/unlock/status?machine_id=$machineId")
            .header("Authorization", "Bearer $accountToken")
            .get()
            .build()
        client.newCall(req).execute().use { resp ->
            if (!resp.isSuccessful) return UnlockStatus(false, null)
            val json = JsonParser.parseString(resp.body?.string() ?: "{}").asJsonObject
            val pending = json.get("pending")?.asBoolean ?: false
            val since = if (json.has("since") && !json.get("since").isJsonNull) json.get("since").asLong else null
            return UnlockStatus(pending, since)
        }
    }

    /** Signale que la machine a ete deverrouillee (efface l'etat d'attente). */
    fun clear(accountToken: String, machineId: String) {
        val body = gson.toJson(mapOf("machine_id" to machineId)).toRequestBody(JSON)
        val req = Request.Builder()
            .url("$workerUrl/unlock/clear")
            .header("Authorization", "Bearer $accountToken")
            .post(body)
            .build()
        client.newCall(req).execute().use { }
    }
}
