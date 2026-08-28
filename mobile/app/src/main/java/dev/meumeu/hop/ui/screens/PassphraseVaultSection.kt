package dev.meumeu.hop.ui.screens

import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import dev.meumeu.hop.unlock.BiometricGate
import dev.meumeu.hop.unlock.UnlockVault

/**
 * Gestion du coffre biometrique de la passphrase LUKS : enregistrer / effacer.
 * Partage entre l'onglet Unlock et l'ecran Compte pour eviter la duplication.
 *
 * Toujours opt-in : rien n'est stocke tant que l'utilisateur ne le demande pas.
 */
@Composable
fun PassphraseVaultSection(
    targetId: String,
    modifier: Modifier = Modifier,
) {
    val context = LocalContext.current
    var hasVault by remember(targetId) { mutableStateOf(UnlockVault.hasPassphrase(context, targetId)) }
    var showSaveDialog by remember { mutableStateOf(false) }
    var showClearConfirm by remember { mutableStateOf(false) }
    var message by remember { mutableStateOf<String?>(null) }
    val biometricAvailable = BiometricGate.isAvailable(context)

    Column(modifier = modifier) {
        Text(
            if (hasVault) "Passphrase enregistrée, scellée par ton empreinte"
            else "Aucune passphrase enregistrée — tu la tapes à chaque fois",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
        Spacer(Modifier.height(8.dp))

        if (hasVault) {
            OutlinedButton(
                onClick = { showClearConfirm = true },
                modifier = Modifier.fillMaxWidth()
            ) {
                Text("Effacer la passphrase")
            }
        } else {
            OutlinedButton(
                onClick = { showSaveDialog = true },
                enabled = biometricAvailable,
                modifier = Modifier.fillMaxWidth()
            ) {
                Text(
                    if (biometricAvailable) "Enregistrer la passphrase"
                    else "Biométrie indisponible sur cet appareil"
                )
            }
        }

        message?.let {
            Spacer(Modifier.height(8.dp))
            Text(it, style = MaterialTheme.typography.bodySmall)
        }
    }

    if (showSaveDialog) {
        var passphrase by remember { mutableStateOf("") }
        AlertDialog(
            onDismissRequest = { showSaveDialog = false },
            title = { Text("Enregistrer la passphrase") },
            text = {
                Column {
                    Text(
                        "Elle sera chiffrée par une clé du Keystore matériel du téléphone, " +
                        "utilisable uniquement avec ton empreinte. Elle ne quitte jamais l'appareil " +
                        "et reste effaçable à tout moment.",
                        style = MaterialTheme.typography.bodySmall
                    )
                    Spacer(Modifier.height(12.dp))
                    OutlinedTextField(
                        value = passphrase,
                        onValueChange = { passphrase = it },
                        label = { Text("Passphrase LUKS") },
                        singleLine = true,
                        visualTransformation = PasswordVisualTransformation()
                    )
                }
            },
            confirmButton = {
                TextButton(
                    enabled = passphrase.isNotEmpty(),
                    onClick = {
                        val toStore = passphrase
                        showSaveDialog = false
                        try {
                            BiometricGate.authenticate(
                                context = context,
                                cipher = UnlockVault.encryptCipher(targetId),
                                title = "Sceller la passphrase",
                                subtitle = "Confirme avec ton empreinte",
                                onSuccess = { cipher ->
                                    UnlockVault.store(context, targetId, cipher, toStore)
                                    hasVault = true
                                    message = "Passphrase enregistrée ✓"
                                },
                                onError = { msg -> message = "Échec : $msg" }
                            )
                        } catch (e: Exception) {
                            message = "Échec : ${e.message ?: e.javaClass.simpleName}"
                        }
                    }
                ) { Text("Enregistrer") }
            },
            dismissButton = {
                TextButton(onClick = { showSaveDialog = false }) { Text("Annuler") }
            }
        )
    }

    if (showClearConfirm) {
        AlertDialog(
            onDismissRequest = { showClearConfirm = false },
            title = { Text("Effacer la passphrase ?") },
            text = {
                Text(
                    "La passphrase enregistrée et sa clé du Keystore seront supprimées. " +
                    "Tu devras la taper à la main au prochain déverrouillage.",
                    style = MaterialTheme.typography.bodySmall
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    UnlockVault.clear(context, targetId)
                    hasVault = false
                    message = "Passphrase effacée"
                    showClearConfirm = false
                }) { Text("Effacer") }
            },
            dismissButton = {
                TextButton(onClick = { showClearConfirm = false }) { Text("Annuler") }
            }
        )
    }
}
