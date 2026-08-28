package dev.meumeu.hop.ssh

import dev.meumeu.hop.network.CfAccessTunnel
import net.schmizz.sshj.SSHClient
import net.schmizz.sshj.connection.channel.direct.Session
import net.schmizz.sshj.transport.verification.PromiscuousVerifier
import java.io.File

/**
 * Session interactive vers dropbear (initramfs) a travers CfAccessTunnel.
 * dropbear force la commande `cryptroot-unlock` sur la cle dediee de l'app
 * (authorized_keys, command=) : le shell demande ici est intercepte par
 * dropbear et remplace par ce prompt de passphrase — cette classe ne voit
 * jamais la passphrase autrement que comme des octets qu'elle relaie entre
 * l'UI et le socket, exactement comme un terminal ferait.
 */
class UnlockSshSession(
    private val hostname: String,
    private val serviceTokenId: String,
    private val serviceTokenSecret: String,
    private val privateKeyFile: File,
    private val expectedHostKey: String = "",
) {
    private var tunnel: CfAccessTunnel? = null
    private var ssh: SSHClient? = null
    private var session: Session? = null
    private var shell: Session.Shell? = null

    var onOutput: ((ByteArray) -> Unit)? = null
    var onClosed: (() -> Unit)? = null
    var onError: ((Throwable) -> Unit)? = null

    fun connect() {
        val t = CfAccessTunnel(hostname, serviceTokenId, serviceTokenSecret)
        tunnel = t
        t.start(onError = { e -> onError?.invoke(e) })

        // start() est asynchrone (le WS finit de s'etablir en tache de fond) ;
        // le ServerSocket local est deja bind et accepte, sshj peut s'y
        // connecter immediatement, les octets sont bufferises cote tunnel.
        val client = SSHClient()
        // dropbear NE regenere PAS sa cle hote (elle est persistee dans
        // /etc/dropbear/initramfs/ et embarquee dans l'image) : on peut donc
        // l'epingler. Sans epinglage, un tiers detenant le secret du tunnel
        // pourrait se faire passer pour la machine et capturer la passphrase.
        if (expectedHostKey.isNotBlank()) {
            client.addHostKeyVerifier(PinnedHostKeyVerifier(expectedHostKey))
        } else {
            client.addHostKeyVerifier(PromiscuousVerifier())
        }
        client.connectTimeout = 15_000
        // Sans ceci, si le WebSocket CF Access n'arrive jamais a s'etablir,
        // le socket local reste ouvert mais muet et l'echange de banniere SSH
        // bloque indefiniment (l'utilisateur voit "Connexion..." pour toujours).
        client.timeout = 20_000
        client.connect("127.0.0.1", t.localPort)
        client.authPublickey("root", client.loadKeys(privateKeyFile.absolutePath))
        ssh = client

        val sess = client.startSession()
        session = sess
        sess.allocateDefaultPTY()
        val sh = sess.startShell() // intercepte par le command= force de dropbear
        shell = sh

        Thread({
            try {
                val input = sh.inputStream
                val buf = ByteArray(4096)
                while (true) {
                    val n = input.read(buf)
                    if (n < 0) break
                    onOutput?.invoke(buf.copyOf(n))
                }
            } catch (_: Exception) {
                // Fin de flux : quand cryptroot-unlock reussit, dropbear coupe
                // la connexion et sshj leve une exception de fermeture (souvent
                // sans message). Ce n'est PAS une erreur — la fin du thread de
                // lecture signifie simplement que la session est terminee.
                // Les vrais echecs de connexion sont remontes par connect().
            } finally {
                onClosed?.invoke()
            }
        }, "unlock-ssh-reader").apply { isDaemon = true }.start()
    }

    fun sendInput(text: String) {
        shell?.outputStream?.let {
            it.write(text.toByteArray())
            it.flush()
        }
    }

    fun disconnect() {
        try { shell?.close() } catch (_: Exception) {}
        try { session?.close() } catch (_: Exception) {}
        try { ssh?.disconnect() } catch (_: Exception) {}
        tunnel?.stop()
    }
}
