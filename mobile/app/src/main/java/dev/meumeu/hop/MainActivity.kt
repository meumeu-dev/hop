package dev.meumeu.hop

import android.net.Uri
import android.os.Bundle
import android.util.Log
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.ActivityResultLauncher
import androidx.fragment.app.FragmentActivity
import androidx.compose.foundation.layout.*
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import com.journeyapps.barcodescanner.ScanContract
import com.journeyapps.barcodescanner.ScanIntentResult
import com.journeyapps.barcodescanner.ScanOptions
import dev.meumeu.hop.network.UnlockClient
import dev.meumeu.hop.ui.HopViewModel
import dev.meumeu.hop.ui.screens.*
import dev.meumeu.hop.ui.theme.HopTheme
import dev.meumeu.hop.unlock.UnlockTarget
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch

class MainActivity : FragmentActivity() {

    private lateinit var qrLauncher: ActivityResultLauncher<ScanOptions>
    private var onQRResult: ((String) -> Unit)? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()

        qrLauncher = registerForActivityResult(ScanContract()) { result: ScanIntentResult ->
            val content = result.contents
            if (content != null) {
                Log.i("HOP", "QR scanned: ${content.take(30)}...")
                onQRResult?.invoke(content)
            }
        }

        setContent {
            HopTheme {
                HopApp(
                    onLaunchQR = { callback ->
                        onQRResult = callback
                        val options = ScanOptions()
                        options.setDesiredBarcodeFormats(ScanOptions.QR_CODE)
                        options.setPrompt("Scanne le QR code affiche par hop pair")
                        options.setBeepEnabled(false)
                        options.setOrientationLocked(true)
                        qrLauncher.launch(options)
                    }
                )
            }
        }
    }
}

sealed class Screen {
    data object Machines : Screen()
    data object Pairing : Screen()
    data object Account : Screen()
    data class Send(val machineName: String) : Screen()
    data class Receive(val machineName: String) : Screen()
    data object Unlock : Screen()
    data class UnlockTerminal(val target: UnlockTarget) : Screen()
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HopApp(
    viewModel: HopViewModel = viewModel(),
    onLaunchQR: ((String) -> Unit) -> Unit = {}
) {
    val state by viewModel.state.collectAsState()
    var currentScreen by remember { mutableStateOf<Screen>(Screen.Machines) }
    var selectedTab by remember { mutableIntStateOf(0) }
    val scope = rememberCoroutineScope()
    val context = androidx.compose.ui.platform.LocalContext.current

    // Machines configurees par l'utilisateur + leur etat d'attente
    var unlockTargets by remember { mutableStateOf(HopConfig(context).loadUnlockTargets().toList()) }
    var unlockChecking by remember { mutableStateOf(false) }
    var unlockStatuses by remember { mutableStateOf<Map<String, String>>(emptyMap()) }
    var unlockSyncing by remember { mutableStateOf(false) }
    var unlockSyncMessage by remember { mutableStateOf<String?>(null) }

    // Sauvegarde/restauration de la config d'unlock via le compte hop. Le blob
    // est chiffre ICI avec la cle derivee du mot de passe (session.dataKey) :
    // le Worker ne stocke qu'un opaque qu'il ne peut pas lire.
    fun pushUnlockConfig() {
        val session = HopConfig(context).loadSession() ?: return
        unlockSyncing = true
        unlockSyncMessage = null
        scope.launch(Dispatchers.IO) {
            unlockSyncMessage = try {
                val json = com.google.gson.Gson().toJson(unlockTargets)
                val encrypted = dev.meumeu.hop.network.AccountClient.encryptData(
                    json.toByteArray(Charsets.UTF_8), session.dataKey
                )
                dev.meumeu.hop.network.AccountClient().pushUnlockConfig(session.token, encrypted)
                "Sauvegardé (${unlockTargets.size} machine(s))"
            } catch (e: Exception) {
                "Échec : ${e.message ?: e.javaClass.simpleName}"
            }
            unlockSyncing = false
        }
    }

    fun forgetAccountBackup() {
        val session = HopConfig(context).loadSession() ?: return
        unlockSyncing = true
        scope.launch(Dispatchers.IO) {
            unlockSyncMessage = try {
                dev.meumeu.hop.network.AccountClient().deleteUnlockConfig(session.token)
                "Sauvegarde supprimée du compte"
            } catch (e: Exception) {
                "Échec : ${e.message ?: e.javaClass.simpleName}"
            }
            unlockSyncing = false
        }
    }

    fun pullUnlockConfig() {
        val session = HopConfig(context).loadSession() ?: return
        unlockSyncing = true
        unlockSyncMessage = null
        scope.launch(Dispatchers.IO) {
            unlockSyncMessage = try {
                val blob = dev.meumeu.hop.network.AccountClient().pullUnlockConfig(session.token)
                if (blob.isEmpty()) {
                    "Aucune sauvegarde sur ce compte"
                } else {
                    val decrypted = dev.meumeu.hop.network.AccountClient.decryptData(blob, session.dataKey)
                    val type = object : com.google.gson.reflect.TypeToken<List<UnlockTarget>>() {}.type
                    val remote: List<UnlockTarget> =
                        com.google.gson.Gson().fromJson(String(decrypted, Charsets.UTF_8), type)
                    // Fusion par id : le distant complete/remplace, on ne perd
                    // pas les machines locales absentes de la sauvegarde.
                    val cfg = HopConfig(context)
                    val merged = cfg.loadUnlockTargets()
                    for (r in remote) {
                        val idx = merged.indexOfFirst { it.id == r.id }
                        if (idx >= 0) merged[idx] = r else merged.add(r)
                    }
                    cfg.saveUnlockTargets(merged)
                    unlockTargets = merged.toList()
                    "Importé (${remote.size} machine(s))"
                }
            } catch (e: Exception) {
                "Échec : ${e.message ?: e.javaClass.simpleName}"
            }
            unlockSyncing = false
        }
    }

    fun refreshUnlockStatus() {
        val session = HopConfig(context).loadSession()
        if (session == null) {
            unlockStatuses = unlockTargets.associate {
                it.machineId to "Connecte-toi à ton compte hop pour voir l'état"
            }
            return
        }
        unlockChecking = true
        scope.launch(Dispatchers.IO) {
            val results = mutableMapOf<String, String>()
            for (t in unlockTargets) {
                results[t.machineId] = try {
                    val st = UnlockClient().status(session.token, t.machineId)
                    if (st.pending) "En attente de déverrouillage" else "Aucun déverrouillage en attente"
                } catch (_: Exception) {
                    "Impossible de joindre le serveur"
                }
            }
            unlockStatuses = results
            unlockChecking = false
        }
    }

    val snackbarHostState = remember { SnackbarHostState() }

    LaunchedEffect(state.error) {
        state.error?.let {
            snackbarHostState.showSnackbar(it, duration = SnackbarDuration.Long)
            viewModel.clearError()
        }
    }

    LaunchedEffect(state.message) {
        state.message?.let {
            snackbarHostState.showSnackbar(it, duration = SnackbarDuration.Short)
            viewModel.clearMessage()
        }
    }

    Scaffold(
        snackbarHost = { SnackbarHost(snackbarHostState) },
        topBar = {
            TopAppBar(
                title = {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text("Hop")
                        Spacer(Modifier.width(8.dp))
                        Text(
                            "v${viewModel.getAppVersion()}",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onPrimaryContainer.copy(alpha = 0.6f)
                        )
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primaryContainer,
                    titleContentColor = MaterialTheme.colorScheme.onPrimaryContainer
                )
            )
        },
        bottomBar = {
            if (currentScreen is Screen.Machines || currentScreen is Screen.Pairing ||
                currentScreen is Screen.Account || currentScreen is Screen.Unlock
            ) {
                NavigationBar {
                    NavigationBarItem(
                        icon = { Icon(Icons.Default.DevicesOther, contentDescription = null) },
                        label = { Text("Machines") },
                        selected = selectedTab == 0,
                        onClick = {
                            selectedTab = 0
                            currentScreen = Screen.Machines
                        }
                    )
                    NavigationBarItem(
                        icon = { Icon(Icons.Default.Link, contentDescription = null) },
                        label = { Text("Pairing") },
                        selected = selectedTab == 1,
                        onClick = {
                            selectedTab = 1
                            currentScreen = Screen.Pairing
                        }
                    )
                    NavigationBarItem(
                        icon = { Icon(Icons.Default.AccountCircle, contentDescription = null) },
                        label = { Text("Compte") },
                        selected = selectedTab == 2,
                        onClick = {
                            selectedTab = 2
                            currentScreen = Screen.Account
                        }
                    )
                    NavigationBarItem(
                        icon = { Icon(Icons.Default.LockOpen, contentDescription = null) },
                        label = { Text("Unlock") },
                        selected = selectedTab == 3,
                        onClick = {
                            selectedTab = 3
                            currentScreen = Screen.Unlock
                            refreshUnlockStatus()
                        }
                    )
                }
            }
        }
    ) { padding ->
        Box(modifier = Modifier.padding(padding)) {
            when (val screen = currentScreen) {
                is Screen.Machines -> MachinesScreen(
                    machines = state.config.machines,
                    machineStatuses = state.machineStatuses,
                    isCheckingMachines = state.isCheckingMachines,
                    appVersion = viewModel.getAppVersion(),
                    onSendTo = { currentScreen = Screen.Send(it) },
                    onReceiveFrom = { currentScreen = Screen.Receive(it) },
                    onRemove = { viewModel.removeMachine(it) },
                    onRefresh = { viewModel.refreshStatuses() }
                )

                is Screen.Pairing -> PairingScreen(
                    isPairing = state.isPairing,
                    pairingCode = state.pairingCode,
                    pairingStatus = state.pairingStatus,
                    pairingToken = state.pairingToken,
                    onStartHost = { viewModel.startPairingHost() },
                    onJoin = { token -> viewModel.joinPairing(token) },
                    onScanQR = {
                        onLaunchQR { content ->
                            viewModel.onQRCodeScanned(content)
                        }
                    }
                )

                is Screen.Account -> AccountScreen(
                    isLoggedIn = state.isLoggedIn,
                    username = state.accountUsername,
                    email = state.accountEmail,
                    isSyncing = state.isSyncing,
                    syncStatus = state.syncStatus,
                    appVersion = viewModel.getAppVersion(),
                    updateAvailable = state.updateAvailable,
                    updateVersion = state.updateVersion,
                    isUpdating = state.isUpdating,
                    onLogin = { email, password -> viewModel.login(email, password) },
                    onRegister = { email, username, password -> viewModel.register(email, username, password) },
                    onLogout = { viewModel.logout() },
                    onSync = { viewModel.sync() },
                    onUpdate = { viewModel.doUpdate() },
                    onCheckUpdate = { viewModel.forceCheckUpdate() }
                )

                is Screen.Send -> SendScreen(
                    machineName = screen.machineName,
                    isTransferring = state.isTransferring,
                    transferStatus = state.transferStatus,
                    onSendFile = { uri: Uri -> viewModel.sendFile(screen.machineName, uri) },
                    onBack = {
                        currentScreen = Screen.Machines
                        selectedTab = 0
                    }
                )

                is Screen.Receive -> ReceiveScreen(
                    machineName = screen.machineName,
                    isTransferring = state.isTransferring,
                    transferStatus = state.transferStatus,
                    onReceive = { path -> viewModel.receiveFile(screen.machineName, path) },
                    onBack = {
                        currentScreen = Screen.Machines
                        selectedTab = 0
                    }
                )

                is Screen.Unlock -> UnlockScreen(
                    targets = unlockTargets,
                    statusByMachine = unlockStatuses,
                    isChecking = unlockChecking,
                    onRefresh = { refreshUnlockStatus() },
                    onUnlock = { target -> currentScreen = Screen.UnlockTerminal(target) },
                    onSave = { target ->
                        HopConfig(context).upsertUnlockTarget(target)
                        unlockTargets = HopConfig(context).loadUnlockTargets().toList()
                    },
                    isLoggedIn = state.isLoggedIn,
                    syncMessage = unlockSyncMessage,
                    isSyncing = unlockSyncing,
                    onPushToAccount = { pushUnlockConfig() },
                    onPullFromAccount = { pullUnlockConfig() },
                    onForgetAccountBackup = { forgetAccountBackup() },
                    onDelete = { target ->
                        dev.meumeu.hop.unlock.UnlockVault.clear(context, target.id)
                        target.deleteKeyFile(context)
                        HopConfig(context).removeUnlockTarget(target.id)
                        unlockTargets = HopConfig(context).loadUnlockTargets().toList()
                    }
                )

                is Screen.UnlockTerminal -> UnlockTerminalScreen(
                    target = screen.target,
                    onUnlocked = {
                        val session = HopConfig(context).loadSession()
                        if (session != null) {
                            scope.launch(Dispatchers.IO) {
                                try { UnlockClient().clear(session.token, screen.target.machineId) } catch (_: Exception) {}
                            }
                        }
                        unlockStatuses = unlockStatuses + (screen.target.machineId to "Aucun déverrouillage en attente")
                    },
                    onBack = {
                        currentScreen = Screen.Unlock
                        selectedTab = 3
                    }
                )
            }
        }
    }
}
