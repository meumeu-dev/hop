package dev.meumeu.hop.ui.screens

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowBack
import androidx.compose.material.icons.filled.Fingerprint
import androidx.compose.material.icons.filled.Visibility
import androidx.compose.material.icons.filled.VisibilityOff
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import dev.meumeu.hop.ssh.UnlockSshSession
import dev.meumeu.hop.unlock.BiometricGate
import dev.meumeu.hop.unlock.UnlockTarget
import dev.meumeu.hop.unlock.UnlockVault
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Terminal brut vers dropbear (prompt cryptroot-unlock), a travers
 * CfAccessTunnel + UnlockSshSession. Buffer texte simple pour le MVP (pas de
 * rendu ANSI).
 *
 * Aucune passphrase n'est stockee : le texte saisi part directement dans le
 * flux SSH puis le champ est vide, comme un vrai terminal.
 */
@Composable
fun UnlockTerminalScreen(
    target: UnlockTarget,
    onUnlocked: () -> Unit,
    onBack: () -> Unit,
) {
    val machineId = target.machineId
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val scrollState = rememberScrollState()

    var output by remember { mutableStateOf("") }
    var input by remember { mutableStateOf("") }
    var status by remember { mutableStateOf("Connexion...") }
    var session by remember { mutableStateOf<UnlockSshSession?>(null) }
    var showPassphrase by remember { mutableStateOf(false) }
    var unlocked by remember { mutableStateOf(false) }
    // Retenue en memoire uniquement, le temps de proposer l'enregistrement
    // apres un unlock reussi. Effacee des que la question est tranchee.
    var lastTypedPassphrase by remember { mutableStateOf<String?>(null) }
    var offerSave by remember { mutableStateOf(false) }
    var saveMessage by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(target.id) {
        withContext(Dispatchers.IO) {
            val keyFile = target.writePrivateKeyFile(context)
            val s = UnlockSshSession(
                hostname = target.hostname,
                serviceTokenId = target.serviceTokenId,
                serviceTokenSecret = target.serviceTokenSecret,
                privateKeyFile = keyFile,
                expectedHostKey = target.hostKey,
            )
            s.onOutput = { bytes ->
                // Buffer borne : une session d'unlock tient en quelques
                // centaines d'octets, mais on evite une croissance illimitee
                // (et la concatenation quadratique qui va avec) si la machine
                // se met a cracher des logs.
                output = (output + String(bytes, Charsets.UTF_8)).takeLast(8000)
                if (output.contains("set up successfully")) {
                    unlocked = true
                    status = "Déverrouillé ✓"
                    // Proposer l'enregistrement seulement si la passphrase
                    // vient d'etre tapee a la main et qu'aucun coffre n'existe.
                    if (lastTypedPassphrase != null &&
                        !UnlockVault.hasPassphrase(context, target.id) &&
                        BiometricGate.isAvailable(context)
                    ) {
                        offerSave = true
                    }
                }
            }
            s.onError = { e -> status = "Erreur: ${e.message ?: e.javaClass.simpleName}" }
            s.onClosed = {
                if (unlocked) {
                    status = "Déverrouillé ✓ — session fermée"
                    onUnlocked()
                } else {
                    status = "Session terminée"
                }
            }
            try {
                s.connect()
                status = "Connecté"
                session = s
            } catch (e: Exception) {
                status = "Connexion échouée: ${e.message}"
            }
        }
    }

    DisposableEffect(Unit) {
        onDispose {
            session?.disconnect()
            // La cle privee est ecrite en clair sur le disque pour sshj :
            // on l'efface des la fin de la session pour reduire la fenetre
            // pendant laquelle elle traine (la source chiffree reste dans
            // unlock_targets.enc et sera reecrite au prochain besoin).
            target.deleteKeyFile(context)
        }
    }

    LaunchedEffect(output) {
        scrollState.animateScrollTo(scrollState.maxValue)
    }

    fun send() {
        val toSend = input
        input = ""
        lastTypedPassphrase = toSend
        scope.launch(Dispatchers.IO) { session?.sendInput(toSend + "\n") }
    }

    /** Deverrouille via le coffre biometrique : rien a taper. */
    fun sendFromVault() {
        val cipher = UnlockVault.decryptCipher(context, target.id)
        if (cipher == null) {
            status = "Coffre vide ou clé invalidée — saisis la passphrase"
            return
        }
        BiometricGate.authenticate(
            context = context,
            cipher = cipher,
            title = "Déverrouiller $machineId",
            subtitle = "Confirme avec ton empreinte",
            onSuccess = { unlockedCipher ->
                val passphrase = UnlockVault.retrieve(context, target.id, unlockedCipher)
                if (passphrase == null) {
                    status = "Impossible de lire la passphrase"
                } else {
                    scope.launch(Dispatchers.IO) { session?.sendInput(passphrase + "\n") }
                }
            },
            onError = { msg -> status = "Biométrie : $msg" }
        )
    }

    if (offerSave) {
        AlertDialog(
            onDismissRequest = {
                offerSave = false
                lastTypedPassphrase = null
            },
            title = { Text("Enregistrer cette passphrase ?") },
            text = {
                Text(
                    "La prochaine fois, tu déverrouilleras d'une simple empreinte, " +
                    "sans rien taper.\n\n" +
                    "Elle serait chiffrée par une clé du Keystore matériel du téléphone, " +
                    "utilisable uniquement avec ton empreinte, et ne quitterait jamais l'appareil. " +
                    "Tu peux l'effacer à tout moment depuis l'onglet Unlock.",
                    style = MaterialTheme.typography.bodySmall
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    val toStore = lastTypedPassphrase
                    offerSave = false
                    if (toStore != null) {
                        try {
                            BiometricGate.authenticate(
                                context = context,
                                cipher = UnlockVault.encryptCipher(target.id),
                                title = "Sceller la passphrase",
                                subtitle = "Confirme avec ton empreinte",
                                onSuccess = { cipher ->
                                    UnlockVault.store(context, target.id, cipher, toStore)
                                    saveMessage = "Passphrase enregistrée ✓"
                                    lastTypedPassphrase = null
                                },
                                onError = { msg ->
                                    saveMessage = "Non enregistrée : $msg"
                                    lastTypedPassphrase = null
                                }
                            )
                        } catch (e: Exception) {
                            saveMessage = "Non enregistrée : ${e.message ?: e.javaClass.simpleName}"
                            lastTypedPassphrase = null
                        }
                    }
                }) { Text("Enregistrer") }
            },
            dismissButton = {
                TextButton(onClick = {
                    offerSave = false
                    lastTypedPassphrase = null
                }) { Text("Non merci") }
            }
        )
    }

    Column(modifier = Modifier.fillMaxSize()) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(8.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            IconButton(onClick = onBack) {
                Icon(Icons.Default.ArrowBack, "Retour")
            }
            Column {
                Text(machineId, style = MaterialTheme.typography.titleMedium)
                Text(saveMessage ?: status, style = MaterialTheme.typography.bodySmall)
            }
        }

        Box(
            modifier = Modifier
                .weight(1f)
                .fillMaxWidth()
                .padding(horizontal = 12.dp)
                .verticalScroll(scrollState)
        ) {
            Text(
                text = output.ifEmpty { "..." },
                fontFamily = FontFamily.Monospace,
                style = MaterialTheme.typography.bodySmall
            )
        }

        if (UnlockVault.hasPassphrase(context, target.id) && !unlocked) {
            Button(
                onClick = { sendFromVault() },
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 12.dp)
            ) {
                Icon(Icons.Default.Fingerprint, contentDescription = null)
                Spacer(Modifier.width(8.dp))
                Text("Déverrouiller avec l'empreinte")
            }
        }

        Row(
            modifier = Modifier.fillMaxWidth().padding(12.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            OutlinedTextField(
                value = input,
                onValueChange = { input = it },
                modifier = Modifier.weight(1f),
                label = { Text("Passphrase") },
                singleLine = true,
                visualTransformation = if (showPassphrase) VisualTransformation.None
                                       else PasswordVisualTransformation(),
                trailingIcon = {
                    IconButton(onClick = { showPassphrase = !showPassphrase }) {
                        Icon(
                            if (showPassphrase) Icons.Default.VisibilityOff else Icons.Default.Visibility,
                            contentDescription = if (showPassphrase) "Masquer" else "Afficher"
                        )
                    }
                },
                keyboardOptions = KeyboardOptions(
                    imeAction = ImeAction.Send,
                    keyboardType = if (showPassphrase) KeyboardType.Text else KeyboardType.Password
                ),
                keyboardActions = KeyboardActions(onSend = { send() })
            )
            Spacer(Modifier.width(8.dp))
            Button(onClick = { send() }) {
                Text("Envoyer")
            }
        }
    }
}
