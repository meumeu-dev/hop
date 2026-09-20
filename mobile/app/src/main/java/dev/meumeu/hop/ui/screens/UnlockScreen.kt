package dev.meumeu.hop.ui.screens

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import dev.meumeu.hop.network.WorkerMachine

/**
 * Liste des machines decouvertes automatiquement via le Worker hop-pair
 * (registry webunlock:machines), auth par le compte hop. L'utilisateur se
 * contente de se connecter a son compte dans l'onglet Compte : plus rien a
 * configurer ici.
 */
@Composable
fun UnlockScreen(
    machines: List<WorkerMachine>,
    statusByMachine: Map<String, String>,
    isChecking: Boolean,
    isLoggedIn: Boolean,
    onRefresh: () -> Unit,
    onUnlock: (WorkerMachine) -> Unit,
    onGoToAccount: () -> Unit,
) {
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
            if (machines.isNotEmpty()) {
                TextButton(onClick = onRefresh, enabled = !isChecking) {
                    Text(if (isChecking) "..." else "Actualiser")
                }
            }
        }

        Spacer(Modifier.height(12.dp))

        if (!isLoggedIn) {
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
                Text("Connecte-toi à ton compte hop", style = MaterialTheme.typography.titleMedium)
                Spacer(Modifier.height(8.dp))
                Text(
                    "Le worker découvre tes machines et s'occupe de tout : " +
                    "tu n'as plus rien à configurer.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
                Spacer(Modifier.height(16.dp))
                Button(onClick = onGoToAccount) {
                    Text("Se connecter")
                }
                Spacer(Modifier.height(8.dp))
                TextButton(onClick = onRefresh, enabled = !isChecking) {
                    Text("Actualiser")
                }
            }
        } else if (machines.isEmpty()) {
            Column(
                modifier = Modifier.fillMaxWidth().weight(1f),
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.Center
            ) {
                Spacer(Modifier.height(16.dp))
                Text("Aucune machine trouvée", style = MaterialTheme.typography.titleMedium)
                Spacer(Modifier.height(8.dp))
                Text(
                    "Le worker ne connaît pas encore de machine à déverrouiller.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
                Spacer(Modifier.height(16.dp))
                Button(onClick = onRefresh, enabled = !isChecking) {
                    Icon(Icons.Default.Refresh, contentDescription = null)
                    Spacer(Modifier.width(8.dp))
                    Text("Actualiser")
                }
            }
        } else {
            LazyColumn(
                verticalArrangement = Arrangement.spacedBy(12.dp),
                modifier = Modifier.weight(1f)
            ) {
                items(machines, key = { it.machineId }) { machine ->
                    Card(modifier = Modifier.fillMaxWidth()) {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(horizontal = 16.dp, vertical = 12.dp),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Column(modifier = Modifier.weight(1f)) {
                                Text(machine.machineId, fontWeight = FontWeight.Bold)
                                Text(
                                    machine.hostname,
                                    style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant
                                )
                                statusByMachine[machine.machineId]?.let {
                                    Text(it, style = MaterialTheme.typography.bodySmall)
                                }
                            }
                            Button(onClick = { onUnlock(machine) }) {
                                Text("Déverrouiller")
                            }
                        }
                    }
                }
            }
        }
    }
}