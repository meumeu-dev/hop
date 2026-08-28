package dev.meumeu.hop.ui.screens

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import dev.meumeu.hop.unlock.UnlockTarget
import java.util.UUID

/**
 * Formulaire de configuration d'une machine a deverrouiller. Toutes les
 * valeurs sont propres a l'utilisateur : rien n'est prerempli avec des
 * secrets, rien n'est code en dur dans l'app.
 */
@Composable
fun UnlockTargetForm(
    initial: UnlockTarget?,
    onDismiss: () -> Unit,
    onConfirm: (UnlockTarget) -> Unit,
) {
    var machineId by remember { mutableStateOf(initial?.machineId ?: "") }
    var hostname by remember { mutableStateOf(initial?.hostname ?: "") }
    var tokenId by remember { mutableStateOf(initial?.serviceTokenId ?: "") }
    var tokenSecret by remember { mutableStateOf(initial?.serviceTokenSecret ?: "") }
    var privateKey by remember { mutableStateOf(initial?.privateKeyPem ?: "") }
    var hostKey by remember { mutableStateOf(initial?.hostKey ?: "") }
    var error by remember { mutableStateOf<String?>(null) }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(if (initial == null) "Ajouter une machine" else "Modifier ${initial.machineId}") },
        text = {
            Column(modifier = Modifier.verticalScroll(rememberScrollState())) {
                Text(
                    "Ces informations restent sur ton téléphone, chiffrées. " +
                    "Elles servent à joindre TA machine via TON tunnel Cloudflare.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
                Spacer(Modifier.height(16.dp))

                OutlinedTextField(
                    value = machineId,
                    onValueChange = { machineId = it },
                    label = { Text("Nom de la machine") },
                    placeholder = { Text("mon-serveur") },
                    supportingText = { Text("Doit correspondre au nom utilisé par le trigger initramfs") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth()
                )
                Spacer(Modifier.height(12.dp))

                OutlinedTextField(
                    value = hostname,
                    onValueChange = { hostname = it },
                    label = { Text("Hostname du tunnel") },
                    placeholder = { Text("unlock-mon-serveur.exemple.com") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth()
                )
                Spacer(Modifier.height(12.dp))

                OutlinedTextField(
                    value = tokenId,
                    onValueChange = { tokenId = it },
                    label = { Text("Service token — Client ID") },
                    placeholder = { Text("xxxxx.access") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth()
                )
                Spacer(Modifier.height(12.dp))

                OutlinedTextField(
                    value = tokenSecret,
                    onValueChange = { tokenSecret = it },
                    label = { Text("Service token — Client Secret") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth()
                )
                Spacer(Modifier.height(12.dp))

                OutlinedTextField(
                    value = privateKey,
                    onValueChange = { privateKey = it },
                    label = { Text("Clé privée SSH (OpenSSH)") },
                    placeholder = { Text("-----BEGIN OPENSSH PRIVATE KEY-----") },
                    supportingText = { Text("La clé publique doit être dans authorized_keys de dropbear, avec command=\"/usr/bin/cryptroot-unlock\"") },
                    textStyle = MaterialTheme.typography.bodySmall.copy(fontFamily = FontFamily.Monospace),
                    minLines = 3,
                    maxLines = 6,
                    modifier = Modifier.fillMaxWidth()
                )

                Spacer(Modifier.height(12.dp))
                OutlinedTextField(
                    value = hostKey,
                    onValueChange = { hostKey = it },
                    label = { Text("Clé hôte SSH de la machine") },
                    placeholder = { Text("ssh-ed25519 AAAA...") },
                    supportingText = { Text("Donnée par `hop unlock setup` (host_key). Sans elle, la machine n'est pas authentifiée.") },
                    textStyle = MaterialTheme.typography.bodySmall.copy(fontFamily = FontFamily.Monospace),
                    minLines = 2,
                    maxLines = 3,
                    modifier = Modifier.fillMaxWidth()
                )

                error?.let {
                    Spacer(Modifier.height(12.dp))
                    Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
                }
            }
        },
        confirmButton = {
            TextButton(onClick = {
                val target = UnlockTarget(
                    id = initial?.id ?: UUID.randomUUID().toString(),
                    machineId = machineId.trim(),
                    hostname = hostname.trim(),
                    serviceTokenId = tokenId.trim(),
                    serviceTokenSecret = tokenSecret.trim(),
                    privateKeyPem = privateKey.trim(),
                    hostKey = hostKey.trim(),
                )
                val problem = target.validate()
                if (problem != null) error = problem else onConfirm(target)
            }) { Text("Enregistrer") }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text("Annuler") }
        }
    )
}
