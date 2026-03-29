package dev.meumeu.hop

import android.net.Uri
import android.os.Bundle
import android.util.Log
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.ActivityResultLauncher
import androidx.compose.foundation.layout.*
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.lifecycle.viewmodel.compose.viewModel
import com.journeyapps.barcodescanner.ScanContract
import com.journeyapps.barcodescanner.ScanIntentResult
import com.journeyapps.barcodescanner.ScanOptions
import dev.meumeu.hop.ui.HopViewModel
import dev.meumeu.hop.ui.screens.*
import dev.meumeu.hop.ui.theme.HopTheme

class MainActivity : ComponentActivity() {

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
                        options.setOrientationLocked(false)
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
                title = { Text("Hop") },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primaryContainer,
                    titleContentColor = MaterialTheme.colorScheme.onPrimaryContainer
                )
            )
        },
        bottomBar = {
            if (currentScreen is Screen.Machines || currentScreen is Screen.Pairing || currentScreen is Screen.Account) {
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
                }
            }
        }
    ) { padding ->
        Box(modifier = Modifier.padding(padding)) {
            when (val screen = currentScreen) {
                is Screen.Machines -> MachinesScreen(
                    machines = state.config.machines,
                    onSendTo = { currentScreen = Screen.Send(it) },
                    onReceiveFrom = { currentScreen = Screen.Receive(it) },
                    onRemove = { viewModel.removeMachine(it) }
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
                    onLogin = { email, password -> viewModel.login(email, password) },
                    onRegister = { email, username, password -> viewModel.register(email, username, password) },
                    onLogout = { viewModel.logout() },
                    onSync = { viewModel.sync() }
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
            }
        }
    }
}
