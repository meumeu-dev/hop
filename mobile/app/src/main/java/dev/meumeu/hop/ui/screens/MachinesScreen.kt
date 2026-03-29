package dev.meumeu.hop.ui.screens

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.meumeu.hop.MachineConfig

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MachinesScreen(
    machines: Map<String, MachineConfig>,
    machineVersions: Map<String, String>,
    isCheckingVersions: Boolean,
    appVersion: String,
    onSendTo: (String) -> Unit,
    onReceiveFrom: (String) -> Unit,
    onRemove: (String) -> Unit,
    onRefreshVersions: () -> Unit
) {
    if (machines.isEmpty()) {
        Box(
            modifier = Modifier.fillMaxSize(),
            contentAlignment = Alignment.Center
        ) {
            Column(horizontalAlignment = Alignment.CenterHorizontally) {
                Icon(
                    Icons.Default.DevicesOther,
                    contentDescription = null,
                    modifier = Modifier.size(64.dp),
                    tint = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.5f)
                )
                Spacer(Modifier.height(16.dp))
                Text(
                    "Aucune machine",
                    style = MaterialTheme.typography.titleMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
                Text(
                    "Utilise l'onglet Pairing pour ajouter",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.7f)
                )
            }
        }
    } else {
        Column(modifier = Modifier.fillMaxSize()) {
            // Refresh bar
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp, vertical = 8.dp),
                horizontalArrangement = Arrangement.End,
                verticalAlignment = Alignment.CenterVertically
            ) {
                if (isCheckingVersions) {
                    CircularProgressIndicator(
                        modifier = Modifier.size(16.dp),
                        strokeWidth = 2.dp
                    )
                    Spacer(Modifier.width(8.dp))
                    Text(
                        "Verification...",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                }
                Spacer(Modifier.weight(1f))
                IconButton(
                    onClick = onRefreshVersions,
                    enabled = !isCheckingVersions
                ) {
                    Icon(
                        Icons.Default.Refresh,
                        contentDescription = "Rafraichir les versions"
                    )
                }
            }

            LazyColumn(
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(16.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                items(machines.entries.toList()) { (name, machine) ->
                    MachineCard(
                        name = name,
                        machine = machine,
                        version = machineVersions[name],
                        appVersion = appVersion,
                        onSend = { onSendTo(name) },
                        onReceive = { onReceiveFrom(name) },
                        onRemove = { onRemove(name) }
                    )
                }
            }
        }
    }
}

@Composable
fun MachineCard(
    name: String,
    machine: MachineConfig,
    version: String?,
    appVersion: String,
    onSend: () -> Unit,
    onReceive: () -> Unit,
    onRemove: () -> Unit
) {
    var showDelete by remember { mutableStateOf(false) }

    Card(
        modifier = Modifier.fillMaxWidth(),
        elevation = CardDefaults.cardElevation(defaultElevation = 2.dp)
    ) {
        Column(modifier = Modifier.padding(16.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Column(modifier = Modifier.weight(1f)) {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        Text(
                            name,
                            style = MaterialTheme.typography.titleMedium,
                            fontWeight = FontWeight.Bold
                        )
                        VersionBadge(version = version, appVersion = appVersion)
                    }
                    Text(
                        "${machine.user}@${machine.ip}",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                    if (machine.tunnel != null) {
                        Text(
                            "Tunnel: ${machine.tunnel}",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.primary
                        )
                    }
                }

                IconButton(onClick = { showDelete = !showDelete }) {
                    Icon(Icons.Default.MoreVert, contentDescription = "Options")
                }
            }

            Spacer(Modifier.height(12.dp))

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Button(
                    onClick = onSend,
                    modifier = Modifier.weight(1f)
                ) {
                    Icon(Icons.Default.Upload, contentDescription = null, modifier = Modifier.size(18.dp))
                    Spacer(Modifier.width(4.dp))
                    Text("Envoyer")
                }

                OutlinedButton(
                    onClick = onReceive,
                    modifier = Modifier.weight(1f)
                ) {
                    Icon(Icons.Default.Download, contentDescription = null, modifier = Modifier.size(18.dp))
                    Spacer(Modifier.width(4.dp))
                    Text("Recevoir")
                }
            }

            if (showDelete) {
                Spacer(Modifier.height(8.dp))
                TextButton(
                    onClick = onRemove,
                    colors = ButtonDefaults.textButtonColors(contentColor = MaterialTheme.colorScheme.error)
                ) {
                    Icon(Icons.Default.Delete, contentDescription = null, modifier = Modifier.size(18.dp))
                    Spacer(Modifier.width(4.dp))
                    Text("Supprimer")
                }
            }
        }
    }
}

@Composable
private fun VersionBadge(version: String?, appVersion: String) {
    if (version == null) return

    val (label, backgroundColor, textColor) = when {
        version == "offline" -> Triple(
            "offline",
            MaterialTheme.colorScheme.error.copy(alpha = 0.15f),
            MaterialTheme.colorScheme.error
        )
        version == "?" || version == "not installed" -> Triple(
            if (version == "not installed") "no hop" else "?",
            MaterialTheme.colorScheme.onSurface.copy(alpha = 0.1f),
            MaterialTheme.colorScheme.onSurfaceVariant
        )
        isOlderVersion(version, appVersion) -> Triple(
            "v$version",
            Color(0xFFFFF3E0), // light orange
            Color(0xFFE65100)  // dark orange
        )
        else -> Triple(
            "v$version",
            Color(0xFFE8F5E9), // light green
            Color(0xFF2E7D32)  // dark green
        )
    }

    Surface(
        shape = MaterialTheme.shapes.small,
        color = backgroundColor
    ) {
        Text(
            text = label,
            modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp),
            style = MaterialTheme.typography.labelSmall.copy(
                fontWeight = FontWeight.Medium,
                fontSize = 10.sp
            ),
            color = textColor
        )
    }
}

/**
 * Compare two semver strings. Returns true if [remote] is strictly older than [local].
 * Non-parseable versions return false (not considered older).
 */
private fun isOlderVersion(remote: String, local: String): Boolean {
    val remoteParts = remote.split("-")[0].split(".").mapNotNull { it.toIntOrNull() }
    val localParts = local.split("-")[0].split(".").mapNotNull { it.toIntOrNull() }
    if (remoteParts.size < 3 || localParts.size < 3) return false
    for (i in 0..2) {
        if (remoteParts[i] < localParts[i]) return true
        if (remoteParts[i] > localParts[i]) return false
    }
    return false
}
