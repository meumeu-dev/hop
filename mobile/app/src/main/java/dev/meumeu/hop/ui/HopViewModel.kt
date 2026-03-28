package dev.meumeu.hop.ui

import android.app.Application
import android.net.Uri
import android.os.Build
import android.provider.OpenableColumns
import android.util.Log
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import dev.meumeu.hop.HopConfig
import dev.meumeu.hop.HopConfigData
import dev.meumeu.hop.MachineConfig
import dev.meumeu.hop.crypto.HopCrypto
import dev.meumeu.hop.network.LanPairing
import dev.meumeu.hop.network.PairData
import dev.meumeu.hop.network.PairSession
import dev.meumeu.hop.network.PairingClient
import dev.meumeu.hop.ssh.SshManager
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import java.io.File
import java.net.InetAddress
import java.net.NetworkInterface
import java.security.MessageDigest

private const val TAG = "HOP"

data class HopUiState(
    val config: HopConfigData = HopConfigData(),
    val pairingCode: String? = null,
    val pairingToken: String? = null,
    val pairingStatus: String = "",
    val isPairing: Boolean = false,
    val transferStatus: String = "",
    val isTransferring: Boolean = false,
    val error: String? = null,
    val message: String? = null
)

class HopViewModel(application: Application) : AndroidViewModel(application) {

    private val hopConfig = HopConfig(application)
    private val sshManager = SshManager()
    private val _state = MutableStateFlow(HopUiState())
    val state: StateFlow<HopUiState> = _state

    init {
        Log.i(TAG, "Hop Android starting")
        loadConfig()
        ensureKeys()
    }

    private fun loadConfig() {
        _state.value = _state.value.copy(config = hopConfig.load())
    }

    private fun ensureKeys() {
        if (!hopConfig.hasKeys()) {
            viewModelScope.launch(Dispatchers.IO) {
                try {
                    Log.i(TAG, "Generating SSH keys...")
                    sshManager.generateEd25519KeyPair(
                        hopConfig.privateKeyFile,
                        hopConfig.publicKeyFile
                    )
                    Log.i(TAG, "SSH keys generated: ${hopConfig.publicKeyFile.absolutePath}")
                } catch (e: Exception) {
                    Log.e(TAG, "Key generation failed", e)
                    _state.value = _state.value.copy(error = "Erreur generation cles: ${e.message}")
                }
            }
        }
    }

    fun clearError() {
        _state.value = _state.value.copy(error = null)
    }

    fun clearMessage() {
        _state.value = _state.value.copy(message = null)
    }

    /** Write token to file accessible via adb */
    private fun writeTokenForAdb(token: String) {
        try {
            val context = getApplication<Application>()
            val tokenFile = File(context.getExternalFilesDir(null), "pairing_token.txt")
            tokenFile.writeText(token)
            Log.i(TAG, "Token written to: ${tokenFile.absolutePath}")
            Log.i(TAG, "adb shell cat ${tokenFile.absolutePath}")
        } catch (e: Exception) {
            Log.w(TAG, "Failed to write token file: ${e.message}")
        }
    }

    // --- Pairing: host mode (relay) ---
    fun startPairingHost() {
        val code = HopCrypto.generateCode()
        Log.i(TAG, "Pairing host mode, code: $code")
        _state.value = _state.value.copy(
            pairingCode = code,
            isPairing = true,
            pairingStatus = "Enregistrement sur le relay..."
        )

        viewModelScope.launch(Dispatchers.IO) {
            try {
                val pubKey = hopConfig.getPublicKey() ?: throw Exception("Cle publique manquante")
                val hostname = Build.MODEL.replace(" ", "-")
                val localIPs = getLocalIPs()
                Log.i(TAG, "Local IPs: $localIPs, hostname: $hostname")

                val pairData = PairData(
                    hostname = hostname,
                    ips = localIPs,
                    ip = localIPs.firstOrNull(),
                    user = "hop",
                    publicKey = pubKey,
                    version = "2.1.0-android"
                )

                val workerUrl = hopConfig.load().workerUrl
                Log.i(TAG, "Worker URL: $workerUrl")
                val client = PairingClient(workerUrl)
                val session = client.publishPairData(code, pairData)

                val fullToken = "${session.pairId}.$code.${session.token}"
                Log.i(TAG, "Pairing token: $fullToken")
                writeTokenForAdb(fullToken)

                _state.value = _state.value.copy(
                    pairingToken = fullToken,
                    pairingStatus = "En attente de l'autre machine..."
                )

                // Race: relay poll + LAN broadcast
                val resultData = raceRelayAndLan(client, session, code, pairData)

                savePairedMachine(resultData)

                Log.i(TAG, "Pairing success: ${resultData.hostname}")
                _state.value = _state.value.copy(
                    isPairing = false,
                    pairingCode = null,
                    pairingToken = null,
                    pairingStatus = "",
                    message = "Paire avec ${resultData.hostname}"
                )
            } catch (e: Exception) {
                Log.e(TAG, "Pairing host failed", e)
                _state.value = _state.value.copy(
                    isPairing = false,
                    pairingCode = null,
                    pairingToken = null,
                    error = "Pairing echoue: ${e.message}"
                )
            }
        }
    }

    /** Race relay polling and LAN broadcast — first to respond wins */
    private fun raceRelayAndLan(
        client: PairingClient,
        session: PairSession,
        code: String,
        myData: PairData
    ): PairData {
        val result = java.util.concurrent.LinkedBlockingQueue<PairData>(1)

        val relayThread = Thread {
            try {
                val response = client.waitForResponse(session, 120_000)
                result.offer(response)
                Log.i(TAG, "Relay response received")
            } catch (e: Exception) {
                Log.d(TAG, "Relay poll ended: ${e.message}")
            }
        }

        val lanThread = Thread {
            try {
                val response = LanPairing.hostLAN(code, myData, 120_000)
                result.offer(response)
                Log.i(TAG, "LAN response received")
            } catch (e: Exception) {
                Log.d(TAG, "LAN host ended: ${e.message}")
            }
        }

        relayThread.start()
        lanThread.start()

        val winner = result.poll(125, java.util.concurrent.TimeUnit.SECONDS)
            ?: throw Exception("Timeout: aucune reponse recue")

        // Interrupt the other
        relayThread.interrupt()
        lanThread.interrupt()
        client.cleanup(session)

        return winner
    }

    // --- Pairing: join via token (relay) ---
    fun joinPairing(token: String) {
        Log.i(TAG, "Joining pairing with token: ${token.take(20)}...")
        _state.value = _state.value.copy(
            isPairing = true,
            pairingStatus = "Connexion..."
        )

        viewModelScope.launch(Dispatchers.IO) {
            try {
                val parts = token.split(".", limit = 3)
                if (parts.size != 3) throw Exception("Token invalide (format: pair_id.code.token)")

                val pairId = parts[0]
                val code = parts[1]
                val workerToken = parts[2]

                val workerUrl = hopConfig.load().workerUrl
                val client = PairingClient(workerUrl)

                _state.value = _state.value.copy(pairingStatus = "Recuperation des donnees...")
                val hostData = client.fetchPairData(pairId, code)
                Log.i(TAG, "Found host: ${hostData.hostname} (${hostData.ip})")

                _state.value = _state.value.copy(
                    pairingStatus = "Trouve: ${hostData.hostname}\nEnvoi de nos infos..."
                )

                val pubKey = hopConfig.getPublicKey() ?: throw Exception("Cle publique manquante")
                val hostname = Build.MODEL.replace(" ", "-")
                val localIPs = getLocalIPs()

                val myData = PairData(
                    hostname = hostname,
                    ips = localIPs,
                    ip = localIPs.firstOrNull(),
                    user = "hop",
                    publicKey = pubKey,
                    version = "2.1.0-android"
                )

                val session = PairSession(pairId = pairId, token = workerToken, code = code)
                client.sendResponse(session, myData)
                Log.i(TAG, "Response sent to ${hostData.hostname}")

                savePairedMachine(hostData)

                _state.value = _state.value.copy(
                    isPairing = false,
                    pairingStatus = "",
                    message = "Paire avec ${hostData.hostname}"
                )
            } catch (e: Exception) {
                Log.e(TAG, "Join pairing failed", e)
                _state.value = _state.value.copy(
                    isPairing = false,
                    error = "Pairing echoue: ${e.message}"
                )
            }
        }
    }

    // --- Pairing: join via LAN code ---
    fun joinPairingLAN(code: String) {
        Log.i(TAG, "Joining LAN pairing with code: $code")
        _state.value = _state.value.copy(
            isPairing = true,
            pairingStatus = "Ecoute des broadcasts LAN..."
        )

        viewModelScope.launch(Dispatchers.IO) {
            try {
                val pubKey = hopConfig.getPublicKey() ?: throw Exception("Cle publique manquante")
                val hostname = Build.MODEL.replace(" ", "-")
                val localIPs = getLocalIPs()

                val myData = PairData(
                    hostname = hostname,
                    ips = localIPs,
                    ip = localIPs.firstOrNull(),
                    user = "hop",
                    publicKey = pubKey,
                    version = "2.1.0-android"
                )

                val hostData = LanPairing.joinLAN(code, myData, 120_000)
                Log.i(TAG, "LAN pairing found: ${hostData.hostname}")

                savePairedMachine(hostData)

                _state.value = _state.value.copy(
                    isPairing = false,
                    pairingStatus = "",
                    message = "Paire avec ${hostData.hostname} (LAN)"
                )
            } catch (e: Exception) {
                Log.e(TAG, "LAN join failed", e)
                _state.value = _state.value.copy(
                    isPairing = false,
                    error = "Pairing LAN echoue: ${e.message}"
                )
            }
        }
    }

    // --- QR code scanned ---
    fun onQRCodeScanned(content: String) {
        Log.i(TAG, "QR scanned: ${content.take(30)}...")
        if (content.contains(".") && content.split(".").size == 3) {
            joinPairing(content)
        } else if (content.length == 8 && content.all { it.isLetterOrDigit() }) {
            joinPairingLAN(content)
        } else {
            _state.value = _state.value.copy(error = "QR code invalide")
        }
    }

    private fun savePairedMachine(data: PairData) {
        val machine = MachineConfig(
            ip = data.ip ?: data.ips?.firstOrNull() ?: "",
            user = data.user,
            tunnel = data.cfDomain?.let { "${data.hostname}.ssh.$it" },
            ips = data.ips
        )
        hopConfig.addMachine(data.hostname, machine)
        Log.i(TAG, "Machine saved: ${data.hostname} (${machine.ip})")

        if (data.cfDomain != null) {
            val cfg = hopConfig.load()
            hopConfig.save(cfg.copy(cfDomain = data.cfDomain))
        }

        loadConfig()
    }

    // --- File transfer: send ---
    fun sendFile(machineName: String, fileUri: Uri) {
        val context = getApplication<Application>()
        Log.i(TAG, "Sending file to $machineName from URI: $fileUri")
        _state.value = _state.value.copy(
            isTransferring = true,
            transferStatus = "Preparation..."
        )

        viewModelScope.launch(Dispatchers.IO) {
            try {
                val config = hopConfig.load()
                val machine = config.machines[machineName]
                    ?: throw Exception("Machine '$machineName' non trouvee")

                val fileName = getFileName(fileUri) ?: "file"
                val tempFile = File(context.cacheDir, fileName)
                context.contentResolver.openInputStream(fileUri)?.use { input ->
                    tempFile.outputStream().use { output ->
                        input.copyTo(output)
                    }
                } ?: throw Exception("Impossible de lire le fichier")

                val fileSize = tempFile.length()
                Log.i(TAG, "Sending $fileName (${formatSize(fileSize)}) to $machineName")
                _state.value = _state.value.copy(
                    transferStatus = "Envoi de $fileName (${formatSize(fileSize)})..."
                )

                val host = findReachableHost(machine)
                Log.i(TAG, "Target host: $host")
                val start = System.currentTimeMillis()

                sshManager.sendFile(
                    host = host,
                    port = 22,
                    user = machine.user,
                    privateKeyFile = hopConfig.privateKeyFile,
                    localFile = tempFile,
                    remotePath = "hop-received/"
                ).getOrThrow()

                val elapsed = (System.currentTimeMillis() - start) / 1000.0
                val speed = if (elapsed > 0) fileSize / elapsed / 1024 / 1024 else 0.0

                val localMd5 = md5(tempFile)
                val remoteMd5 = sshManager.checkRemoteMD5(
                    host = host,
                    port = 22,
                    user = machine.user,
                    privateKeyFile = hopConfig.privateKeyFile,
                    remotePath = "hop-received/$fileName"
                ).getOrNull()

                val integrity = if (localMd5 == remoteMd5) " - MD5 OK" else ""
                tempFile.delete()

                Log.i(TAG, "Send complete: $fileName (${String.format("%.1f", speed)} MB/s)$integrity")
                _state.value = _state.value.copy(
                    isTransferring = false,
                    transferStatus = "",
                    message = "$fileName envoye (${String.format("%.1f", speed)} MB/s)$integrity"
                )
            } catch (e: Exception) {
                Log.e(TAG, "Send failed", e)
                _state.value = _state.value.copy(
                    isTransferring = false,
                    error = "Envoi echoue: ${e.message}"
                )
            }
        }
    }

    // --- File transfer: receive ---
    fun receiveFile(machineName: String, remotePath: String) {
        val context = getApplication<Application>()
        Log.i(TAG, "Receiving $remotePath from $machineName")
        _state.value = _state.value.copy(
            isTransferring = true,
            transferStatus = "Reception..."
        )

        viewModelScope.launch(Dispatchers.IO) {
            try {
                val config = hopConfig.load()
                val machine = config.machines[machineName]
                    ?: throw Exception("Machine '$machineName' non trouvee")

                val downloadDir = File(context.getExternalFilesDir(null), "hop-received")
                downloadDir.mkdirs()

                val host = findReachableHost(machine)
                Log.i(TAG, "Target host: $host")
                val start = System.currentTimeMillis()

                val file = sshManager.receiveFile(
                    host = host,
                    port = 22,
                    user = machine.user,
                    privateKeyFile = hopConfig.privateKeyFile,
                    remotePath = remotePath,
                    localDir = downloadDir
                ).getOrThrow()

                val elapsed = (System.currentTimeMillis() - start) / 1000.0
                Log.i(TAG, "Receive complete: ${file.name} (${String.format("%.1f", elapsed)}s)")

                _state.value = _state.value.copy(
                    isTransferring = false,
                    transferStatus = "",
                    message = "${file.name} recu (${String.format("%.1f", elapsed)}s)"
                )
            } catch (e: Exception) {
                Log.e(TAG, "Receive failed", e)
                _state.value = _state.value.copy(
                    isTransferring = false,
                    error = "Reception echouee: ${e.message}"
                )
            }
        }
    }

    fun removeMachine(name: String) {
        Log.i(TAG, "Removing machine: $name")
        hopConfig.removeMachine(name)
        loadConfig()
    }

    // --- Helpers ---

    private fun getLocalIPs(): List<String> {
        val ips = mutableListOf<String>()
        try {
            NetworkInterface.getNetworkInterfaces()?.toList()?.forEach { ni ->
                if (ni.isUp && !ni.isLoopback) {
                    ni.inetAddresses.toList().forEach { addr ->
                        if (addr is java.net.Inet4Address && !addr.isLoopbackAddress) {
                            ips.add(addr.hostAddress ?: "")
                        }
                    }
                }
            }
        } catch (_: Exception) {}
        return ips.sortedByDescending { it.startsWith("192.168") }
    }

    private fun findReachableHost(machine: MachineConfig): String {
        val ipsToTry = (machine.ips ?: listOf(machine.ip)).filter { it.isNotBlank() }
        for (ip in ipsToTry) {
            try {
                if (InetAddress.getByName(ip).isReachable(1500)) return ip
            } catch (_: Exception) {}
        }
        if (machine.tunnel != null) return machine.tunnel
        return machine.ip
    }

    private fun getFileName(uri: Uri): String? {
        val context = getApplication<Application>()
        context.contentResolver.query(uri, null, null, null, null)?.use { cursor ->
            if (cursor.moveToFirst()) {
                val idx = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
                if (idx >= 0) return cursor.getString(idx)
            }
        }
        return uri.lastPathSegment
    }

    private fun md5(file: File): String {
        val digest = MessageDigest.getInstance("MD5")
        file.inputStream().use { input ->
            val buffer = ByteArray(8192)
            var read: Int
            while (input.read(buffer).also { read = it } > 0) {
                digest.update(buffer, 0, read)
            }
        }
        return digest.digest().joinToString("") { "%02x".format(it) }
    }

    private fun formatSize(bytes: Long): String = when {
        bytes >= 1L shl 30 -> "%.1f GB".format(bytes.toDouble() / (1L shl 30))
        bytes >= 1L shl 20 -> "%.1f MB".format(bytes.toDouble() / (1L shl 20))
        bytes >= 1L shl 10 -> "%.1f KB".format(bytes.toDouble() / (1L shl 10))
        else -> "$bytes B"
    }
}
