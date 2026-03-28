package dev.meumeu.hop.ui.screens

import androidx.compose.foundation.layout.*
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Link
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

@Composable
fun PairingScreen(
    isPairing: Boolean,
    pairingCode: String?,
    pairingStatus: String,
    pairingToken: String?,
    onStartHost: () -> Unit,
    onJoin: (token: String) -> Unit
) {
    var joinToken by remember { mutableStateOf("") }
    var mode by remember { mutableStateOf<String?>(null) }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(24.dp),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        if (isPairing) {
            Spacer(Modifier.height(48.dp))
            CircularProgressIndicator(modifier = Modifier.size(48.dp))
            Spacer(Modifier.height(24.dp))

            if (pairingToken != null) {
                Text(
                    "Token de pairing",
                    style = MaterialTheme.typography.titleMedium
                )
                Spacer(Modifier.height(8.dp))

                // Show code prominently
                if (pairingCode != null) {
                    Text(
                        pairingCode,
                        fontSize = 32.sp,
                        fontWeight = FontWeight.Bold,
                        fontFamily = FontFamily.Monospace,
                        color = MaterialTheme.colorScheme.primary,
                        letterSpacing = 4.sp
                    )
                    Spacer(Modifier.height(8.dp))
                }

                // Full token in a selectable text for copy
                SelectionContainer {
                    Text(
                        pairingToken,
                        fontSize = 12.sp,
                        fontFamily = FontFamily.Monospace,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        textAlign = TextAlign.Center
                    )
                }

                Spacer(Modifier.height(16.dp))
                Text(
                    "Sur l'autre machine:\nhop pair <token>",
                    textAlign = TextAlign.Center,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }

            Spacer(Modifier.height(24.dp))
            Text(
                pairingStatus,
                textAlign = TextAlign.Center,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        } else if (mode == null) {
            Spacer(Modifier.height(48.dp))
            Icon(
                Icons.Default.Link,
                contentDescription = null,
                modifier = Modifier.size(64.dp),
                tint = MaterialTheme.colorScheme.primary
            )
            Spacer(Modifier.height(24.dp))
            Text(
                "Pairing",
                style = MaterialTheme.typography.headlineMedium,
                fontWeight = FontWeight.Bold
            )
            Text(
                "Connecte ton telephone a une machine hop",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Spacer(Modifier.height(48.dp))

            Button(
                onClick = { mode = "host" },
                modifier = Modifier.fillMaxWidth()
            ) {
                Text("Generer un code (attendre)")
            }

            Spacer(Modifier.height(16.dp))

            OutlinedButton(
                onClick = { mode = "join" },
                modifier = Modifier.fillMaxWidth()
            ) {
                Text("Entrer un token (rejoindre)")
            }
        } else if (mode == "host") {
            Spacer(Modifier.height(48.dp))
            Text(
                "Generer un code",
                style = MaterialTheme.typography.titleLarge,
                fontWeight = FontWeight.Bold
            )
            Spacer(Modifier.height(16.dp))
            Text(
                "L'autre machine entre le token avec:\nhop pair <token>",
                textAlign = TextAlign.Center,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Spacer(Modifier.height(32.dp))

            Button(
                onClick = onStartHost,
                modifier = Modifier.fillMaxWidth()
            ) {
                Text("Demarrer")
            }

            Spacer(Modifier.height(16.dp))
            TextButton(onClick = { mode = null }) {
                Text("Retour")
            }
        } else {
            // Join mode — enter full token from other machine
            Spacer(Modifier.height(48.dp))
            Text(
                "Rejoindre",
                style = MaterialTheme.typography.titleLarge,
                fontWeight = FontWeight.Bold
            )
            Spacer(Modifier.height(16.dp))
            Text(
                "Colle le token affiche par\n'hop pair' sur l'autre machine",
                textAlign = TextAlign.Center,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Spacer(Modifier.height(32.dp))

            OutlinedTextField(
                value = joinToken,
                onValueChange = { joinToken = it.trim() },
                label = { Text("Token (pair_id.code.token)") },
                modifier = Modifier.fillMaxWidth(),
                singleLine = true,
                textStyle = LocalTextStyle.current.copy(
                    fontFamily = FontFamily.Monospace,
                    fontSize = 14.sp
                )
            )

            Spacer(Modifier.height(24.dp))

            Button(
                onClick = { onJoin(joinToken) },
                modifier = Modifier.fillMaxWidth(),
                enabled = joinToken.contains(".")
            ) {
                Text("Rejoindre")
            }

            Spacer(Modifier.height(16.dp))
            TextButton(onClick = { mode = null }) {
                Text("Retour")
            }
        }
    }
}

@Composable
private fun SelectionContainer(content: @Composable () -> Unit) {
    androidx.compose.foundation.text.selection.SelectionContainer {
        content()
    }
}
