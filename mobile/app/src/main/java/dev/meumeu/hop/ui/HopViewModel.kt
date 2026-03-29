package dev.meumeu.hop.ui

import android.app.Application
import android.net.Uri
import android.os.Build
import android.provider.OpenableColumns
import android.util.Log
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.google.gson.Gson
import dev.meumeu.hop.HopConfig
import dev.meumeu.hop.HopConfigData
import dev.meumeu.hop.MachineConfig
import dev.meumeu.hop.crypto.HopCrypto
import dev.meumeu.hop.network.AccountClient
import dev.meumeu.hop.network.AccountSession
import dev.meumeu.hop.network.AppUpdater
import dev.meumeu.hop.network.LanPairing
import dev.meumeu.hop.network.UpdateInfo
import dev.meumeu.hop.network.PairData
import dev.meumeu.hop.network.PairSession
import dev.meumeu.hop.network.PairingClient
import dev.meumeu.hop.ssh.SshManager
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
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
    val message: String? = null,
    // Account state
    val isLoggedIn: Boolean = false,
    val accountUsername: String? = null,
    val accountEmail: String? = null,
    val isSyncing: Boolean = false,
    val syncStatus: String = "",
    // Update state
    val updateAvailable: Boolean = false,
    val updateVersion: String? = null,
    val isUpdating: Boolean = false,
    // Machine status
    val machineStatuses: Map<String, MachineStatus> = emptyMap(),
    val isCheckingMachines: Boolean = false
)

data class MachineStatus(
    val lanReachable: Boolean = false,
    val tunnelReachable: Boolean = false,
    val version: String = "?",
    val connectedVia: String = "offline" // "lan", "tunnel", "lan+tunnel", "offline"
)

class HopViewModel(application: Application) : AndroidViewModel(application) {

    private val hopConfig = HopConfig(application)
    private val sshManager = SshManager()
    private val _state = MutableStateFlow(HopUiState())
    val state: StateFlow<HopUiState> = _state

    private var accountSession: AccountSession? = null
    private var latestUpdate: UpdateInfo? = null

    init {
        Log.i(TAG, "Hop Android starting")
        clearTokenFile() // Clean stale pairing token from previous crash
        loadConfig()
        ensureKeys()
        loadSession()
        checkForUpdate()
        checkMachineStatuses()
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

    /** Write token to file accessible via adb (internal storage, deleted after use) */
    private fun writeTokenForAdb(token: String) {
        try {
            val context = getApplication<Application>()
            // Use internal storage (not external) — only accessible via adb run-as or root
            val tokenFile = File(context.filesDir, "pairing_token.txt")
            tokenFile.writeText(token)
            Log.i(TAG, "Token written to: ${tokenFile.absolutePath}")
            Log.i(TAG, "adb shell run-as dev.meumeu.hop cat files/pairing_token.txt")
        } catch (e: Exception) {
            Log.w(TAG, "Failed to write token file: ${e.message}")
        }
    }

    private fun clearTokenFile() {
        try {
            val context = getApplication<Application>()
            val tokenFile = File(context.filesDir, "pairing_token.txt")
            tokenFile.delete()
        } catch (_: Exception) {}
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
                    version = "2.2.0-android"
                )

                val workerUrl = hopConfig.load().workerUrl
                Log.i(TAG, "Worker URL: $workerUrl")
                val client = PairingClient(workerUrl)
                val session = client.publishPairData(code, pairData)

                val fullToken = "${session.pairId}.$code.${session.token}"
                Log.i(TAG, "Pairing token generated (${fullToken.take(8)}...)")
                writeTokenForAdb(fullToken)

                _state.value = _state.value.copy(
                    pairingToken = fullToken,
                    pairingStatus = "En attente de l'autre machine..."
                )

                // Race: relay poll + LAN broadcast
                val resultData = raceRelayAndLan(client, session, code, pairData)

                savePairedMachine(resultData)
                clearTokenFile()

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
                clearTokenFile()
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
        Log.i(TAG, "joinPairing raw token: '${token}' (len=${token.length})")
        _state.value = _state.value.copy(
            isPairing = true,
            pairingStatus = "Connexion..."
        )

        viewModelScope.launch(Dispatchers.IO) {
            try {
                val cleaned = token.trim()
                Log.i(TAG, "joinPairing cleaned: '${cleaned.take(60)}' (len=${cleaned.length})")

                val parts = cleaned.split(".", limit = 3)
                Log.i(TAG, "joinPairing parts: ${parts.size} -> [${parts.map { "'${it.take(20)}'" }}]")
                if (parts.size != 3) throw Exception("Token invalide: ${parts.size} parties au lieu de 3")

                val pairId = parts[0].trim()
                val code = parts[1].trim()
                val workerToken = parts[2].trim()

                if (pairId.isEmpty()) throw Exception("pair_id vide")
                if (code.isEmpty()) throw Exception("code vide")
                if (workerToken.isEmpty()) throw Exception("worker token vide")

                Log.i(TAG, "joinPairing pairId=$pairId code=$code tokenLen=${workerToken.length}")

                val workerUrl = hopConfig.load().workerUrl
                Log.i(TAG, "joinPairing worker=$workerUrl")
                val client = PairingClient(workerUrl)

                _state.value = _state.value.copy(pairingStatus = "Recuperation des donnees...")
                Log.i(TAG, "joinPairing fetching pair data...")
                val hostData = client.fetchPairData(pairId, code)
                Log.i(TAG, "joinPairing found: hostname=${hostData.hostname} ip=${hostData.ip}")

                _state.value = _state.value.copy(
                    pairingStatus = "Trouve: ${hostData.hostname}\nEnvoi de nos infos..."
                )

                val pubKey = hopConfig.getPublicKey()
                Log.i(TAG, "joinPairing pubKey=${pubKey?.take(30)}")
                if (pubKey == null) throw Exception("Cle publique manquante — redemarrer l'app")

                val hostname = Build.MODEL.replace(" ", "-")
                val localIPs = getLocalIPs()
                Log.i(TAG, "joinPairing hostname=$hostname ips=$localIPs")

                val myData = PairData(
                    hostname = hostname,
                    ips = localIPs,
                    ip = localIPs.firstOrNull(),
                    user = "hop",
                    publicKey = pubKey,
                    version = getAppVersion() + "-android"
                )

                val session = PairSession(pairId = pairId, token = workerToken, code = code)
                Log.i(TAG, "joinPairing sending response...")
                client.sendResponse(session, myData)
                Log.i(TAG, "joinPairing response sent OK")

                savePairedMachine(hostData)

                _state.value = _state.value.copy(
                    isPairing = false,
                    pairingStatus = "",
                    message = "Paire avec ${hostData.hostname}"
                )
            } catch (e: Exception) {
                Log.e(TAG, "joinPairing FAILED: ${e.javaClass.simpleName}: ${e.message}", e)
                _state.value = _state.value.copy(
                    isPairing = false,
                    error = "Pairing echoue: ${e.javaClass.simpleName}: ${e.message}"
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
                    version = "2.2.0-android"
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
        val trimmed = content.trim()
        Log.i(TAG, "QR scanned: '${trimmed.take(40)}...' (len=${trimmed.length})")
        if (trimmed.contains(".") && trimmed.split(".").size >= 3) {
            // Relay token: pair_id.code.token (token part may contain dots)
            joinPairing(trimmed)
        } else if (trimmed.length == 8 && trimmed.all { it.isLetterOrDigit() }) {
            joinPairingLAN(trimmed)
        } else {
            Log.w(TAG, "Invalid QR content: '${trimmed.take(50)}'")
            _state.value = _state.value.copy(error = "QR code invalide: format non reconnu")
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

    // --- Machine status checking (LAN/tunnel/version) ---

    private fun checkMachineStatuses() {
        val config = hopConfig.load()
        if (config.machines.isEmpty()) return

        _state.value = _state.value.copy(isCheckingMachines = true)

        viewModelScope.launch(Dispatchers.IO) {
            val results = config.machines.map { (name, machine) ->
                async {
                    var lanReachable = false
                    var tunnelReachable = false
                    var version = "?"
                    var connectedHost: String? = null

                    // Check LAN IPs
                    val ipsToTry = (machine.ips ?: listOf(machine.ip)).filter { it.isNotBlank() }
                    for (ip in ipsToTry) {
                        try {
                            if (InetAddress.getByName(ip).isReachable(2000)) {
                                lanReachable = true
                                connectedHost = ip
                                break
                            }
                        } catch (_: Exception) {}
                    }

                    // Check tunnel
                    if (machine.tunnel != null) {
                        try {
                            val addr = InetAddress.getByName(machine.tunnel)
                            if (addr != null) tunnelReachable = true
                        } catch (_: Exception) {}
                    }

                    // Get version via SSH (use LAN if available, tunnel otherwise)
                    val sshHost = connectedHost ?: machine.tunnel
                    if (sshHost != null && (lanReachable || tunnelReachable)) {
                        try {
                            version = sshManager.getRemoteHopVersion(
                                host = sshHost,
                                port = 22,
                                user = machine.user,
                                privateKeyFile = hopConfig.privateKeyFile
                            ).getOrDefault("?")
                        } catch (e: Exception) {
                            Log.d(TAG, "Version check failed for $name: ${e.message}")
                        }
                    }

                    val connectedVia = when {
                        lanReachable && tunnelReachable -> "lan+tunnel"
                        lanReachable -> "lan"
                        tunnelReachable -> "tunnel"
                        else -> "offline"
                    }

                    Log.d(TAG, "$name: $connectedVia (v$version)")
                    name to MachineStatus(
                        lanReachable = lanReachable,
                        tunnelReachable = tunnelReachable,
                        version = version,
                        connectedVia = connectedVia
                    )
                }
            }.awaitAll().toMap()

            Log.i(TAG, "Machine statuses: ${results.map { "${it.key}=${it.value.connectedVia}" }}")
            _state.value = _state.value.copy(
                machineStatuses = results,
                isCheckingMachines = false
            )
        }
    }

    fun refreshStatuses() {
        checkMachineStatuses()
    }

    // --- Account ---

    private fun loadSession() {
        accountSession = hopConfig.loadSession()
        accountSession?.let { session ->
            _state.value = _state.value.copy(
                isLoggedIn = true,
                accountUsername = session.username,
                accountEmail = session.email
            )
            Log.i(TAG, "Session restored for ${session.username}")
        }
    }

    fun login(email: String, password: String) {
        Log.i(TAG, "Logging in: $email")
        _state.value = _state.value.copy(isSyncing = true, syncStatus = "Connexion...")

        viewModelScope.launch(Dispatchers.IO) {
            try {
                val workerUrl = hopConfig.load().workerUrl
                val client = AccountClient(workerUrl)
                val session = client.login(email, password)

                accountSession = session
                hopConfig.saveSession(session)

                _state.value = _state.value.copy(
                    isLoggedIn = true,
                    accountUsername = session.username,
                    accountEmail = session.email,
                    isSyncing = false,
                    syncStatus = "",
                    message = "Connecte: ${session.username}"
                )
                Log.i(TAG, "Login success: ${session.username}")
            } catch (e: Exception) {
                Log.e(TAG, "Login failed", e)
                _state.value = _state.value.copy(
                    isSyncing = false,
                    syncStatus = "",
                    error = "Connexion echouee: ${e.message}"
                )
            }
        }
    }

    fun register(email: String, username: String, password: String) {
        Log.i(TAG, "Registering: $email ($username)")
        _state.value = _state.value.copy(isSyncing = true, syncStatus = "Creation du compte...")

        viewModelScope.launch(Dispatchers.IO) {
            try {
                val workerUrl = hopConfig.load().workerUrl
                val client = AccountClient(workerUrl)
                val session = client.register(email, username, password)

                accountSession = session
                hopConfig.saveSession(session)

                _state.value = _state.value.copy(
                    isLoggedIn = true,
                    accountUsername = session.username,
                    accountEmail = session.email,
                    isSyncing = false,
                    syncStatus = "",
                    message = "Compte cree: ${session.username}"
                )
                Log.i(TAG, "Register success: ${session.username}")
            } catch (e: Exception) {
                Log.e(TAG, "Register failed", e)
                _state.value = _state.value.copy(
                    isSyncing = false,
                    syncStatus = "",
                    error = "Inscription echouee: ${e.message}"
                )
            }
        }
    }

    fun logout() {
        Log.i(TAG, "Logging out")
        val token = accountSession?.token

        viewModelScope.launch(Dispatchers.IO) {
            if (token != null) {
                try {
                    val workerUrl = hopConfig.load().workerUrl
                    AccountClient(workerUrl).logout(token)
                } catch (_: Exception) {}
            }
        }

        accountSession = null
        hopConfig.deleteSession()
        _state.value = _state.value.copy(
            isLoggedIn = false,
            accountUsername = null,
            accountEmail = null,
            message = "Deconnecte"
        )
    }

    fun sync() {
        val session = accountSession ?: return
        Log.i(TAG, "Syncing machines...")
        _state.value = _state.value.copy(isSyncing = true, syncStatus = "Synchronisation...")

        viewModelScope.launch(Dispatchers.IO) {
            try {
                val workerUrl = hopConfig.load().workerUrl
                val client = AccountClient(workerUrl)

                // Push local machines to cloud
                _state.value = _state.value.copy(syncStatus = "Envoi des machines...")
                val config = hopConfig.load()
                val machinesJson = Gson().toJson(config.machines)
                val encrypted = AccountClient.encryptData(
                    machinesJson.toByteArray(Charsets.UTF_8),
                    session.dataKey
                )
                client.pushMachines(session.token, encrypted)

                // Pull machines from cloud and merge
                _state.value = _state.value.copy(syncStatus = "Reception des machines...")
                val remoteEncrypted = client.pullMachines(session.token)

                if (remoteEncrypted.isNotEmpty()) {
                    val decrypted = AccountClient.decryptData(remoteEncrypted, session.dataKey)
                    val type = object : com.google.gson.reflect.TypeToken<Map<String, dev.meumeu.hop.MachineConfig>>() {}.type
                    val remoteMachines: Map<String, dev.meumeu.hop.MachineConfig> =
                        Gson().fromJson(String(decrypted, Charsets.UTF_8), type)

                    // Merge: remote machines that are not local get added
                    val currentConfig = hopConfig.load()
                    var added = 0
                    for ((name, machine) in remoteMachines) {
                        if (!currentConfig.machines.containsKey(name)) {
                            currentConfig.machines[name] = machine
                            added++
                        }
                    }
                    if (added > 0) {
                        hopConfig.save(currentConfig)
                        loadConfig()
                    }

                    Log.i(TAG, "Sync complete: ${remoteMachines.size} remote, $added new")
                    _state.value = _state.value.copy(
                        isSyncing = false,
                        syncStatus = "",
                        message = "Synchronise ($added nouvelles machines)"
                    )
                } else {
                    Log.i(TAG, "Sync complete: no remote data")
                    _state.value = _state.value.copy(
                        isSyncing = false,
                        syncStatus = "",
                        message = "Machines envoyees au cloud"
                    )
                }
            } catch (e: Exception) {
                Log.e(TAG, "Sync failed", e)
                _state.value = _state.value.copy(
                    isSyncing = false,
                    syncStatus = "",
                    error = "Sync echoue: ${e.message}"
                )
            }
        }
    }

    // --- Auto-update ---

    private fun checkForUpdate() {
        viewModelScope.launch(Dispatchers.IO) {
            val context = getApplication<Application>()
            val currentVersion = try {
                context.packageManager.getPackageInfo(context.packageName, 0).versionName ?: "0.0.0"
            } catch (_: Exception) { "0.0.0" }

            val update = AppUpdater.checkUpdate(currentVersion)
            if (update != null && update.hasUpdate) {
                latestUpdate = update
                _state.value = _state.value.copy(
                    updateAvailable = true,
                    updateVersion = update.latestVersion
                )
                Log.i(TAG, "Update available: ${update.latestVersion}")
            }
        }
    }

    fun doUpdate() {
        val update = latestUpdate ?: return
        val context = getApplication<Application>()
        _state.value = _state.value.copy(isUpdating = true)
        Log.i(TAG, "Starting update to ${update.latestVersion}")

        viewModelScope.launch(Dispatchers.IO) {
            try {
                val apkFile = AppUpdater.downloadApk(context, update) { progress ->
                    Log.d(TAG, "Download progress: $progress%")
                }
                // Switch to main thread for install intent
                launch(Dispatchers.Main) {
                    AppUpdater.installApk(context, apkFile)
                    _state.value = _state.value.copy(isUpdating = false)
                }
            } catch (e: Exception) {
                Log.e(TAG, "Update failed", e)
                _state.value = _state.value.copy(
                    isUpdating = false,
                    error = "Mise a jour echouee: ${e.message}"
                )
            }
        }
    }

    fun forceCheckUpdate() {
        viewModelScope.launch(Dispatchers.IO) {
            val context = getApplication<Application>()
            val currentVersion = try {
                context.packageManager.getPackageInfo(context.packageName, 0).versionName ?: "0.0.0"
            } catch (_: Exception) { "0.0.0" }

            val update = AppUpdater.checkUpdate(currentVersion)
            if (update != null && update.hasUpdate) {
                latestUpdate = update
                _state.value = _state.value.copy(
                    updateAvailable = true,
                    updateVersion = update.latestVersion,
                    message = "Mise a jour disponible: v${update.latestVersion}"
                )
            } else {
                _state.value = _state.value.copy(
                    updateAvailable = false,
                    updateVersion = null,
                    message = "Deja a jour (v$currentVersion)"
                )
            }
        }
    }

    fun getAppVersion(): String {
        val context = getApplication<Application>()
        return try {
            context.packageManager.getPackageInfo(context.packageName, 0).versionName ?: "?"
        } catch (_: Exception) { "?" }
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
