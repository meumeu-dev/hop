package dev.meumeu.hop

import com.google.gson.Gson
import com.google.gson.annotations.SerializedName
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.bouncycastle.crypto.generators.Argon2BytesGenerator
import org.bouncycastle.crypto.params.Argon2Parameters
import org.bouncycastle.jce.provider.BouncyCastleProvider
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test
import java.security.SecureRandom
import java.security.Security
import java.util.Base64
import java.util.concurrent.TimeUnit
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

/**
 * Full end-to-end pairing test: simulates what the Android app does
 * by calling the real worker API.
 */
class PairingTest {

    data class PairData(
        val hostname: String? = null,
        val ip: String? = null,
        val ips: List<String>? = null,
        val user: String? = null,
        @SerializedName("public_key") val publicKey: String? = null,
        @SerializedName("host_key") val hostKey: String? = null,
        @SerializedName("cf_domain") val cfDomain: String? = null,
        val version: String? = null
    )

    private val WORKER = "https://hop-pair.meumeudev.workers.dev"
    private val gson = Gson()
    private val client = OkHttpClient.Builder()
        .connectTimeout(30, TimeUnit.SECONDS)
        .readTimeout(30, TimeUnit.SECONDS)
        .build()
    private val jsonType = "application/json".toMediaType()

    @Before
    fun setup() {
        Security.removeProvider("BC")
        Security.addProvider(BouncyCastleProvider())
    }

    private fun deriveKey(code: String, salt: ByteArray): ByteArray {
        val params = Argon2Parameters.Builder(Argon2Parameters.ARGON2_id)
            .withSalt(salt)
            .withIterations(3)
            .withMemoryAsKB(64 * 1024)
            .withParallelism(1)
            .build()
        val gen = Argon2BytesGenerator()
        gen.init(params)
        val key = ByteArray(32)
        gen.generateBytes(code.toByteArray(Charsets.UTF_8), key)
        return key
    }

    private fun decrypt(encoded: String, code: String): ByteArray {
        val raw = Base64.getDecoder().decode(encoded)
        val salt = raw.sliceArray(0 until 16)
        val nonce = raw.sliceArray(16 until 28)
        val ciphertext = raw.sliceArray(28 until raw.size)
        val key = deriveKey(code, salt)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, SecretKeySpec(key, "AES"), GCMParameterSpec(128, nonce))
        return cipher.doFinal(ciphertext)
    }

    private fun encrypt(data: ByteArray, code: String): String {
        val salt = ByteArray(16)
        SecureRandom().nextBytes(salt)
        val key = deriveKey(code, salt)
        val nonce = ByteArray(12)
        SecureRandom().nextBytes(nonce)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, SecretKeySpec(key, "AES"), GCMParameterSpec(128, nonce))
        val ciphertext = cipher.doFinal(data)
        return Base64.getEncoder().encodeToString(salt + nonce + ciphertext)
    }

    @Test
    fun testFullPairingFlow() {
        println("=== Step 1: Create pairing session (simulate hop pair server) ===")

        val code = "test" + (1000..9999).random()
        val serverData = PairData(
            hostname = "test-server",
            ip = "10.0.0.1",
            user = "testuser",
            publicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAItest",
            version = "2.4.7"
        )

        val serverJson = gson.toJson(serverData)
        println("Server data JSON: $serverJson")

        val encrypted = encrypt(serverJson.toByteArray(), code)
        println("Encrypted length: ${encrypted.length}")

        // Publish to worker
        val publishBody = gson.toJson(mapOf("data" to encrypted)).toRequestBody(jsonType)
        val publishReq = Request.Builder().url("$WORKER/pair").post(publishBody).build()
        val publishResp = client.newCall(publishReq).execute()
        val publishResult = gson.fromJson(publishResp.body!!.string(), Map::class.java)
        assertTrue("Publish should succeed", publishResult["ok"] as Boolean)

        val pairId = publishResult["pair_id"] as String
        val workerToken = publishResult["token"] as String
        println("Pair ID: $pairId")
        println("Worker Token: ${workerToken.take(16)}...")

        // Build full token like CLI does
        val fullToken = "$pairId.$code.$workerToken"
        println("Full token: ${fullToken.take(60)}...")

        println("\n=== Step 2: Simulate Android client joining ===")

        // Parse token (like Android does)
        val parts = fullToken.trim().split(".", limit = 3)
        assertEquals("Should have 3 parts", 3, parts.size)

        val clientPairId = parts[0].trim()
        val clientCode = parts[1].trim()
        val clientToken = parts[2].trim()

        println("Parsed: pairId=$clientPairId code=$clientCode tokenLen=${clientToken.length}")

        // Fetch pair data (like Android does)
        val fetchReq = Request.Builder().url("$WORKER/pair/$clientPairId").get().build()
        val fetchResp = client.newCall(fetchReq).execute()
        println("Fetch response code: ${fetchResp.code}")
        assertEquals(200, fetchResp.code)

        val fetchBody = fetchResp.body!!.string()
        println("Fetch body: ${fetchBody.take(100)}")
        val fetchResult = gson.fromJson(fetchBody, Map::class.java)
        val fetchedEncrypted = fetchResult["data"] as? String

        assertNotNull("data field should not be null", fetchedEncrypted)
        println("Fetched encrypted length: ${fetchedEncrypted!!.length}")

        // Decrypt (like Android HopCrypto.decrypt does)
        val decrypted = decrypt(fetchedEncrypted, clientCode)
        val decryptedJson = String(decrypted, Charsets.UTF_8)
        println("Decrypted: $decryptedJson")

        val hostData = gson.fromJson(decryptedJson, PairData::class.java)
        assertEquals("test-server", hostData.hostname)
        println("Host: ${hostData.hostname} (${hostData.user}@${hostData.ip})")

        // Send response (like Android does)
        val clientData = PairData(
            hostname = "test-android",
            ip = "10.0.0.2",
            user = "hop",
            publicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIclient",
            version = "2.4.7-android"
        )
        val clientJson = gson.toJson(clientData)
        val clientEncrypted = encrypt(clientJson.toByteArray(), clientCode)

        val respBody = gson.toJson(mapOf("data" to clientEncrypted)).toRequestBody(jsonType)
        val respReq = Request.Builder()
            .url("$WORKER/pair/$clientPairId/response")
            .post(respBody)
            .header("X-Pair-Token", clientToken)
            .build()
        val respResp = client.newCall(respReq).execute()
        println("Response send code: ${respResp.code}")
        val respResult = gson.fromJson(respResp.body!!.string(), Map::class.java)
        assertTrue("Send response should succeed", respResult["ok"] as Boolean)

        println("\n=== Step 3: Verify server can read client response ===")

        val pollReq = Request.Builder()
            .url("$WORKER/pair/$clientPairId/response")
            .get()
            .header("X-Pair-Token", workerToken)
            .build()
        val pollResp = client.newCall(pollReq).execute()
        assertEquals(200, pollResp.code)
        val pollResult = gson.fromJson(pollResp.body!!.string(), Map::class.java)
        val pollEncrypted = pollResult["data"] as String
        val pollDecrypted = decrypt(pollEncrypted, code)
        val receivedClient = gson.fromJson(String(pollDecrypted), PairData::class.java)
        assertEquals("test-android", receivedClient.hostname)

        println("Server received client: ${receivedClient.hostname} OK!")
        println("\n=== FULL PAIRING FLOW: SUCCESS ===")

        // Cleanup
        val cleanReq = Request.Builder()
            .url("$WORKER/pair/$clientPairId")
            .delete()
            .header("X-Pair-Token", workerToken)
            .build()
        client.newCall(cleanReq).execute()
    }
}
