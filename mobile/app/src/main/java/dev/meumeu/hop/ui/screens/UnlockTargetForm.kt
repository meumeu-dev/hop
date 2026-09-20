package dev.meumeu.hop.ui.screens

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import dev.meumeu.hop.unlock.UnlockTarget
import java.util.UUID

/**
 * Formulaire de configuration d'une machine a deverrouiller. Toutes les
 * valeurs sont propres a l'utilisateur : rien n'est prerempli avec des
 * secrets, rien n'est code en dur dans l'app.
 *
 * Le deverrouillage passe par le serveur WEB d'unlock de la machine
 * (hostname = ingress du tunnel, auth service token). Plus de cle SSH, plus
 * de flux terminal.
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
                    label = { Text("Hostname du tunnel (web d'unlock)") },
                    placeholder = { Text("unlock-web-mon-serveur.exemple.com") },
                    supportingText = { Text("L'ingress du tunnel qui sert la page web d'unlock (port 8088)") },
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