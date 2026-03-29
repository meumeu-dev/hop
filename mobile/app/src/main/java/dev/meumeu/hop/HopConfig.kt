package dev.meumeu.hop

import android.content.Context
import android.provider.Settings
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import dev.meumeu.hop.network.AccountSession
import java.io.File
import java.security.MessageDigest
import java.security.SecureRandom
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

data class MachineConfig(
    val ip: String,
    val user: String,
    val tunnel: String? = null,
    val ips: List<String>? = null
)

data class HopConfigData(
    val machines: MutableMap<String, MachineConfig> = mutableMapOf(),
    val workerUrl: String = "https://hop-pair.meumeudev.workers.dev",
    val cfDomain: String? = null
)

class HopConfig(private val context: Context) {

    private val gson = Gson()
    private val configFile get() = File(context.filesDir, "config.json")
    private val keysDir get() = File(context.filesDir, "keys").also { it.mkdirs() }
    private val sessionFile get() = File(context.filesDir, "session.enc")

    val privateKeyFile get() = File(keysDir, "hop_ed25519")
    val publicKeyFile get() = File(keysDir, "hop_ed25519.pub")

    fun load(): HopConfigData {
        if (!configFile.exists()) return HopConfigData()
        return try {
            gson.fromJson(configFile.readText(), HopConfigData::class.java)
        } catch (_: Exception) {
            HopConfigData()
        }
    }

    fun save(config: HopConfigData) {
        configFile.writeText(gson.toJson(config))
    }

    fun addMachine(name: String, machine: MachineConfig) {
        val config = load()
        config.machines[name] = machine
        save(config)
    }

    fun removeMachine(name: String) {
        val config = load()
        config.machines.remove(name)
        save(config)
    }

    fun hasKeys(): Boolean = privateKeyFile.exists() && publicKeyFile.exists()

    fun getPublicKey(): String? {
        return if (publicKeyFile.exists()) publicKeyFile.readText().trim() else null
    }

    fun reset() {
        configFile.delete()
        keysDir.deleteRecursively()
    }

    // --- Session storage (encrypted with device-specific key) ---

    /**
     * Derive a device-specific encryption key from ANDROID_ID + package name.
     * Mirrors Go's approach: SHA256(hopDir + "hop-session-key").
     */
    private fun deriveLocalKey(): ByteArray {
        val androidId = Settings.Secure.getString(context.contentResolver, Settings.Secure.ANDROID_ID)
            ?: "default-android-id"
        val material = "${context.packageName}:${androidId}:hop-session-key"
        return MessageDigest.getInstance("SHA-256").digest(material.toByteArray(Charsets.UTF_8))
    }

    private fun localEncrypt(data: ByteArray): String {
        val key = deriveLocalKey()
        val nonce = ByteArray(12)
        SecureRandom().nextBytes(nonce)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, SecretKeySpec(key, "AES"), GCMParameterSpec(128, nonce))
        val ciphertext = cipher.doFinal(data)
        return Base64.getEncoder().encodeToString(nonce + ciphertext)
    }

    private fun localDecrypt(encoded: String): ByteArray {
        val key = deriveLocalKey()
        val raw = Base64.getDecoder().decode(encoded)
        require(raw.size > 12) { "encrypted data too short" }
        val nonce = raw.sliceArray(0 until 12)
        val ciphertext = raw.sliceArray(12 until raw.size)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, SecretKeySpec(key, "AES"), GCMParameterSpec(128, nonce))
        return cipher.doFinal(ciphertext)
    }

    fun saveSession(session: AccountSession) {
        val json = gson.toJson(session)
        val encrypted = localEncrypt(json.toByteArray(Charsets.UTF_8))
        sessionFile.writeText(encrypted)
    }

    fun loadSession(): AccountSession? {
        if (!sessionFile.exists()) return null
        return try {
            val encrypted = sessionFile.readText()
            val decrypted = localDecrypt(encrypted)
            gson.fromJson(String(decrypted, Charsets.UTF_8), AccountSession::class.java)
        } catch (_: Exception) {
            null
        }
    }

    fun deleteSession() {
        sessionFile.delete()
    }
}
