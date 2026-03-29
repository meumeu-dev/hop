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
import dev.meumeu.hop.ui.MachineStatus

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MachinesScreen(
    machines: Map<String, MachineConfig>,
    machineStatuses: Map<String, MachineStatus>,
    isCheckingMachines: Boolean,
    appVersion: String,
    onSendTo: (String) -> Unit,
    onReceiveFrom: (String) -> Unit,
    onRemove: (String) -> Unit,
    onRefresh: () -> Unit
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
            // Status bar
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp, vertical = 8.dp),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                if (isCheckingMachines) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        CircularProgressIndicator(
                            modifier = Modifier.size(14.dp),
                            strokeWidth = 2.dp
                        )
                        Spacer(Modifier.width(8.dp))
                        Text(
                            "Scan en cours...",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
                } else {
                    // Summary
                    val online = machineStatuses.count { it.value.connectedVia != "offline" }
                    val total = machines.size
                    Text(
                        "$online/$total en ligne",
                        style = MaterialTheme.typography.bodySmall,
                        color = if (online == total) Color(0xFF2E7D32)
                            else MaterialTheme.colorScheme.onSurfaceVariant
                    )
                }
                IconButton(
                    onClick = onRefresh,
                    enabled = !isCheckingMachines
                ) {
                    Icon(Icons.Default.Refresh, contentDescription = "Rafraichir")
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
                        status = machineStatuses[name],
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
    status: MachineStatus?,
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
                        // Status dot
                        StatusDot(status)
                        Text(
                            name,
                            style = MaterialTheme.typography.titleMedium,
                            fontWeight = FontWeight.Bold
                        )
                        // Version badge
                        if (status != null && status.version != "?") {
                            VersionBadge(version = status.version, appVersion = appVersion)
                        }
                    }
                    Spacer(Modifier.height(4.dp))
                    Text(
                        "${machine.user}@${machine.ip}",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                    // Connection mode chips
                    if (status != null && status.connectedVia != "offline") {
                        Spacer(Modifier.height(4.dp))
                        Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                            if (status.lanReachable) {
                                ConnectionChip("LAN", Color(0xFF2E7D32))
                            }
                            if (status.tunnelReachable) {
                                ConnectionChip("Tunnel", Color(0xFF1565C0))
                            }
                        }
                    } else if (status?.connectedVia == "offline") {
                        Spacer(Modifier.height(4.dp))
                        ConnectionChip("Hors ligne", MaterialTheme.colorScheme.error)
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
                    modifier = Modifier.weight(1f),
                    enabled = status?.connectedVia != "offline"
                ) {
                    Icon(Icons.Default.Upload, contentDescription = null, modifier = Modifier.size(18.dp))
                    Spacer(Modifier.width(4.dp))
                    Text("Envoyer")
                }

                OutlinedButton(
                    onClick = onReceive,
                    modifier = Modifier.weight(1f),
                    enabled = status?.connectedVia != "offline"
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
private fun StatusDot(status: MachineStatus?) {
    val color = when {
        status == null -> MaterialTheme.colorScheme.onSurface.copy(alpha = 0.3f)
        status.connectedVia == "offline" -> MaterialTheme.colorScheme.error
        status.connectedVia == "lan+tunnel" -> Color(0xFF2E7D32)
        status.connectedVia == "lan" -> Color(0xFF4CAF50)
        status.connectedVia == "tunnel" -> Color(0xFF2196F3)
        else -> MaterialTheme.colorScheme.onSurface.copy(alpha = 0.3f)
    }
    Surface(
        modifier = Modifier.size(10.dp),
        shape = MaterialTheme.shapes.extraLarge,
        color = color
    ) {}
}

@Composable
private fun ConnectionChip(label: String, color: Color) {
    Surface(
        shape = MaterialTheme.shapes.small,
        color = color.copy(alpha = 0.12f)
    ) {
        Text(
            text = label,
            modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp),
            style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.sp),
            color = color,
            fontWeight = FontWeight.Medium
        )
    }
}

@Composable
private fun VersionBadge(version: String, appVersion: String) {
    val isOld = isOlderVersion(version, appVersion)
    val bgColor = if (isOld) Color(0xFFFFF3E0) else Color(0xFFE8F5E9)
    val txtColor = if (isOld) Color(0xFFE65100) else Color(0xFF2E7D32)

    Surface(
        shape = MaterialTheme.shapes.small,
        color = bgColor
    ) {
        Text(
            text = "v$version",
            modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp),
            style = MaterialTheme.typography.labelSmall.copy(
                fontWeight = FontWeight.Medium,
                fontSize = 10.sp
            ),
            color = txtColor
        )
    }
}

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
