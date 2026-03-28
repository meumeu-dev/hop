package dev.meumeu.hop

import android.content.Context
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import java.io.File

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
}
