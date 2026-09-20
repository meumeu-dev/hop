package dev.meumeu.hop.ui.screens

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
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
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import dev.meumeu.hop.unlock.BiometricGate
import dev.meumeu.hop.unlock.UnlockTarget
import dev.meumeu.hop.unlock.UnlockVault
import dev.meumeu.hop.unlock.UnlockWebSession
import kotlinx.coroutines.launch

/**
 * Deverrouillage via le serveur WEB d'unlock de la machine : la passphrase est
 * chiffree en RSA-OAEP-256 dans le telephone et envoyee en POST /unlock a
 * travers le tunnel Cloudflare (auth service token). Pas de terminal SSH, pas
 * de flux interactif : on envoie, la machine dechiffre, et on affiche la
 * reponse.
 *
 * Aucune passphrase n'est stockee : le texte saisi part directement dans le
 * chiffrement puis le champ est vide.
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

    var input by remember { mutableStateOf("") }
    var status by remember { mutableStateOf("Prêt à déverrouiller") }
    var busy by remember { mutableStateOf(false) }
    var unlocked by remember { mutableStateOf(false) }
    var showPassphrase by remember { mutableStateOf(false) }
    // Retenue en memoire uniquement, le temps de proposer l'enregistrement
    // apres un unlock reussi. Effacee des que la question est tranchee.
    var lastTypedPassphrase by remember { mutableStateOf<String?>(null) }
    var offerSave by remember { mutableStateOf(false) }
    var saveMessage by remember { mutableStateOf<String?>(null) }

    suspend fun doUnlock(passphrase: String) {
        if (busy) return
        busy = true
        status = "Déverrouillage en cours…"
        val session = UnlockWebSession(
            hostname = target.hostname,
            serviceTokenId = target.serviceTokenId,
            serviceTokenSecret = target.serviceTokenSecret,
        )
        try {
            val result = session.unlock(passphrase)
            if (result.ok) {
                unlocked = true
                status = "✓ ${result.msg}"
                // Proposer l'enregistrement seulement si la passphrase
                // vient d'etre tapee a la main et qu'aucun coffre n'existe.
                if (lastTypedPassphrase != null &&
                    !UnlockVault.hasPassphrase(context, target.id) &&
                    BiometricGate.isAvailable(context)
                ) {
                    offerSave = true
                } else {
                    onUnlocked()
                }
            } else {
                status = "✗ ${result.msg}"
            }
        } catch (e: Exception) {
            status = "Erreur : ${e.message ?: e.javaClass.simpleName}"
        } finally {
            busy = false
        }
    }

    fun send() {
        val toSend = input
        input = ""
        lastTypedPassphrase = toSend
        scope.launch { doUnlock(toSend) }
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
                    lastTypedPassphrase = null
                    scope.launch { doUnlock(passphrase) }
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
                                    onUnlocked()
                                },
                                onError = { msg ->
                                    saveMessage = "Non enregistrée : $msg"
                                    lastTypedPassphrase = null
                                    onUnlocked()
                                }
                            )
                        } catch (e: Exception) {
                            saveMessage = "Non enregistrée : ${e.message ?: e.javaClass.simpleName}"
                            lastTypedPassphrase = null
                            onUnlocked()
                        }
                    } else {
                        onUnlocked()
                    }
                }) { Text("Enregistrer") }
            },
            dismissButton = {
                TextButton(onClick = {
                    offerSave = false
                    lastTypedPassphrase = null
                    onUnlocked()
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
                .padding(horizontal = 12.dp),
            contentAlignment = Alignment.Center
        ) {
            Text(
                "La passphrase est chiffrée dans ce téléphone (RSA-OAEP) puis envoyée " +
                "à $machineId via ton tunnel Cloudflare. Elle n'y transite jamais en clair.",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }

        if (UnlockVault.hasPassphrase(context, target.id) && !unlocked) {
            Button(
                onClick = { sendFromVault() },
                enabled = !busy,
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
                enabled = !busy && !unlocked,
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
                keyboardActions = KeyboardActions(onSend = { if (!busy) send() })
            )
            Spacer(Modifier.width(8.dp))
            Button(onClick = { send() }, enabled = !busy && !unlocked) {
                Text("Envoyer")
            }
        }
    }
}