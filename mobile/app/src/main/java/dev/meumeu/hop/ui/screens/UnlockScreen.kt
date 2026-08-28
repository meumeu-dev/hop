package dev.meumeu.hop.ui.screens

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import dev.meumeu.hop.unlock.UnlockTarget

/**
 * Liste des machines que l'utilisateur peut deverrouiller. Tout est configure
 * ici (hostname du tunnel, service token, cle SSH) — rien n'est code en dur,
 * chaque utilisateur vise ses propres appareils.
 */
@Composable
fun UnlockScreen(
    targets: List<UnlockTarget>,
    statusByMachine: Map<String, String>,
    isChecking: Boolean,
    onRefresh: () -> Unit,
    onUnlock: (UnlockTarget) -> Unit,
    onSave: (UnlockTarget) -> Unit,
    onDelete: (UnlockTarget) -> Unit,
    isLoggedIn: Boolean,
    syncMessage: String?,
    isSyncing: Boolean,
    onPushToAccount: () -> Unit,
    onPullFromAccount: () -> Unit,
    onForgetAccountBackup: () -> Unit,
    onImportQR: () -> Unit,
) {
    var editing by remember { mutableStateOf<UnlockTarget?>(null) }
    var showForm by remember { mutableStateOf(false) }
    var confirmDelete by remember { mutableStateOf<UnlockTarget?>(null) }
    var confirmPush by remember { mutableStateOf(false) }
    var confirmForget by remember { mutableStateOf(false) }

    Column(modifier = Modifier.fillMaxSize().padding(16.dp)) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                "Déverrouillage",
                style = MaterialTheme.typography.titleLarge,
                fontWeight = FontWeight.Bold
            )
            Row {
                if (targets.isNotEmpty()) {
                    TextButton(onClick = onRefresh, enabled = !isChecking) {
                        Text(if (isChecking) "..." else "Actualiser")
                    }
                }
                TextButton(onClick = onImportQR) { Text("Scanner") }
                IconButton(onClick = {
                    editing = null
                    showForm = true
                }) {
                    Icon(Icons.Default.Add, contentDescription = "Ajouter une machine")
                }
            }
        }

        Spacer(Modifier.height(12.dp))

        if (targets.isEmpty()) {
            // weight(1f) et non fillMaxSize() : sinon ce bloc mange toute la
            // hauteur et repousse la section de synchronisation hors de
            // l'ecran — precisement sur un telephone neuf, ou le bouton
            // "Importer" est le seul dont on a besoin.
            Column(
                modifier = Modifier.fillMaxWidth().weight(1f),
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.Center
            ) {
                Icon(
                    Icons.Default.Lock,
                    contentDescription = null,
                    modifier = Modifier.size(64.dp),
                    tint = MaterialTheme.colorScheme.onSurfaceVariant
                )
                Spacer(Modifier.height(16.dp))
                Text("Aucune machine configurée", style = MaterialTheme.typography.titleMedium)
                Spacer(Modifier.height(8.dp))
                Text(
                    "Ajoute une machine avec son tunnel Cloudflare, son service token " +
                    "et sa clé SSH pour la déverrouiller à distance.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
                Spacer(Modifier.height(16.dp))
                Button(onClick = {
                    editing = null
                    showForm = true
                }) {
                    Icon(Icons.Default.Add, contentDescription = null)
                    Spacer(Modifier.width(8.dp))
                    Text("Ajouter une machine")
                }
            }
        } else {
            LazyColumn(
                verticalArrangement = Arrangement.spacedBy(12.dp),
                modifier = Modifier.weight(1f)
            ) {
                items(targets, key = { it.id }) { target ->
                    Card(modifier = Modifier.fillMaxWidth()) {
                        Column(modifier = Modifier.padding(16.dp)) {
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.SpaceBetween,
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Column(modifier = Modifier.weight(1f)) {
                                    Text(target.machineId, fontWeight = FontWeight.Bold)
                                    Text(
                                        target.hostname,
                                        style = MaterialTheme.typography.bodySmall,
                                        color = MaterialTheme.colorScheme.onSurfaceVariant
                                    )
                                    statusByMachine[target.machineId]?.let {
                                        Text(it, style = MaterialTheme.typography.bodySmall)
                                    }
                                }
                                IconButton(onClick = {
                                    editing = target
                                    showForm = true
                                }) {
                                    Icon(Icons.Default.Edit, contentDescription = "Modifier")
                                }
                                IconButton(onClick = { confirmDelete = target }) {
                                    Icon(Icons.Default.Delete, contentDescription = "Supprimer")
                                }
                            }
                            Spacer(Modifier.height(12.dp))
                            Button(
                                onClick = { onUnlock(target) },
                                modifier = Modifier.fillMaxWidth()
                            ) {
                                Text("Déverrouiller")
                            }
                            Spacer(Modifier.height(12.dp))
                            HorizontalDivider()
                            Spacer(Modifier.height(12.dp))
                            PassphraseVaultSection(targetId = target.id)
                        }
                    }
                }
            }
        }

        // --- Synchronisation avec le compte hop ---
        if (isLoggedIn) {
            Spacer(Modifier.height(12.dp))
            HorizontalDivider()
            Spacer(Modifier.height(12.dp))
            Text(
                "Sauvegarde chiffrée sur ton compte hop",
                style = MaterialTheme.typography.bodySmall,
                fontWeight = FontWeight.Bold
            )
            Text(
                "Retrouve tes machines depuis un autre téléphone. Le contenu est " +
                "chiffré avec ta clé de compte : le serveur ne peut pas le lire. " +
                "La passphrase enregistrée, elle, reste sur cet appareil.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Spacer(Modifier.height(8.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedButton(
                    onClick = { confirmPush = true },
                    enabled = !isSyncing && targets.isNotEmpty(),
                    modifier = Modifier.weight(1f)
                ) { Text("Sauvegarder") }
                OutlinedButton(
                    onClick = onPullFromAccount,
                    enabled = !isSyncing,
                    modifier = Modifier.weight(1f)
                ) { Text("Importer") }
            }
            TextButton(
                onClick = { confirmForget = true },
                enabled = !isSyncing
            ) { Text("Supprimer la sauvegarde du compte") }
            syncMessage?.let {
                Spacer(Modifier.height(8.dp))
                Text(it, style = MaterialTheme.typography.bodySmall)
            }
        } else {
            Spacer(Modifier.height(12.dp))
            Text(
                "Connecte-toi à ton compte hop (onglet Compte) pour sauvegarder " +
                "tes machines et les retrouver ailleurs.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
    }

    if (confirmPush) {
        AlertDialog(
            onDismissRequest = { confirmPush = false },
            title = { Text("Sauvegarder sur ton compte ?") },
            text = {
                Text(
                    "Les clés SSH et les service tokens de tes machines seront envoyés, " +
                    "chiffrés avec la clé dérivée de ton mot de passe hop. Le serveur ne " +
                    "peut pas les déchiffrer, mais ils quittent ce téléphone : choisis un " +
                    "mot de passe solide.\n\nLa passphrase LUKS n'est jamais envoyée " +
                    "(elle est scellée dans le matériel de cet appareil).",
                    style = MaterialTheme.typography.bodySmall
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    confirmPush = false
                    onPushToAccount()
                }) { Text("Sauvegarder") }
            },
            dismissButton = {
                TextButton(onClick = { confirmPush = false }) { Text("Annuler") }
            }
        )
    }

    if (confirmForget) {
        AlertDialog(
            onDismissRequest = { confirmForget = false },
            title = { Text("Supprimer la sauvegarde ?") },
            text = {
                Text(
                    "La copie chiffrée stockée sur ton compte sera effacée. " +
                    "Les machines configurées sur CE téléphone ne sont pas touchées.",
                    style = MaterialTheme.typography.bodySmall
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    confirmForget = false
                    onForgetAccountBackup()
                }) { Text("Supprimer") }
            },
            dismissButton = {
                TextButton(onClick = { confirmForget = false }) { Text("Annuler") }
            }
        )
    }

    if (showForm) {
        UnlockTargetForm(
            initial = editing,
            onDismiss = { showForm = false },
            onConfirm = { target ->
                showForm = false
                onSave(target)
            }
        )
    }

    confirmDelete?.let { target ->
        AlertDialog(
            onDismissRequest = { confirmDelete = null },
            title = { Text("Supprimer ${target.machineId} ?") },
            text = {
                Text(
                    "La configuration, la clé SSH et la passphrase enregistrée pour " +
                    "cette machine seront supprimées de ce téléphone.",
                    style = MaterialTheme.typography.bodySmall
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    onDelete(target)
                    confirmDelete = null
                }) { Text("Supprimer") }
            },
            dismissButton = {
                TextButton(onClick = { confirmDelete = null }) { Text("Annuler") }
            }
        )
    }
}
