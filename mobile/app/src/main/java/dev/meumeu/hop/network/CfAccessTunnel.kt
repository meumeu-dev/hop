package dev.meumeu.hop.network

import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString
import okio.ByteString.Companion.toByteString
import java.io.IOException
import java.net.InetAddress
import java.net.ServerSocket
import java.net.Socket
import java.util.concurrent.LinkedBlockingQueue
import java.util.concurrent.TimeUnit

/**
 * Reimplemente cote client le mecanisme de "cloudflared access ssh" : ouvre
 * un WebSocket vers un hostname protege par Cloudflare Access en envoyant
 * les headers Cf-Access-Client-Id/Secret (auth service token, sans
 * navigateur), et expose un socket local 127.0.0.1:<port> sur lequel sshj
 * peut se connecter comme un serveur SSH normal — sshj n'a aucune
 * connaissance de CF Access, il voit juste un serveur SSH local.
 *
 * Aucune passphrase LUKS ne transite dans cette classe elle-meme : c'est un
 * simple tuyau d'octets entre le socket local et le WebSocket distant.
 */
class CfAccessTunnel(
    private val hostname: String,
    private val serviceTokenId: String,
    private val serviceTokenSecret: String,
) {
    private val client = OkHttpClient.Builder()
        .readTimeout(0, TimeUnit.MILLISECONDS) // session interactive longue, pas de timeout
        .pingInterval(20, TimeUnit.SECONDS)
        .build()

    private var webSocket: WebSocket? = null
    private var serverSocket: ServerSocket? = null
    private var clientSocket: Socket? = null
    private var incoming: LinkedBlockingQueue<ByteArray>? = null

    var localPort: Int = -1
        private set

    private val closedMarker = ByteArray(0)

    /**
     * Ouvre le socket local et le WebSocket CF Access. Retourne des que le
     * socket local est pret a accepter (sshj peut s'y connecter tout de
     * suite ; le WS peut finir de s'etablir en parallele, les octets sont
     * bufferises).
     */
    fun start(onError: (Throwable) -> Unit) {
        // Socket d'ecoute sur la boucle locale uniquement. Note de securite :
        // sur Android, n'importe quelle autre app peut se connecter a un port
        // localhost. La fenetre est etroite (le port n'existe que le temps de
        // la session et est ferme des la 1re connexion acceptee), et un
        // intrus n'obtiendrait qu'un canal vers dropbear sans la cle SSH —
        // donc rien de plus qu'une banniere. Backlog volontairement a 1.
        val ss = ServerSocket(0, 1, InetAddress.getByName("127.0.0.1"))
        serverSocket = ss
        localPort = ss.localPort

        val incoming = LinkedBlockingQueue<ByteArray>()
        this.incoming = incoming

        val request = Request.Builder()
            .url("wss://$hostname/")
            .addHeader("Cf-Access-Client-Id", serviceTokenId)
            .addHeader("Cf-Access-Client-Secret", serviceTokenSecret)
            .build()

        val listener = object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                Thread({
                    try {
                        val socket = ss.accept()
                        clientSocket = socket
                        // Une seule session par tunnel : on ferme l'ecoute des
                        // la premiere connexion pour ne pas laisser un port
                        // ouvert derriere nous.
                        try { ss.close() } catch (_: IOException) {}
                        pumpSocketToWebSocket(socket, webSocket)
                        pumpWebSocketToSocket(socket, incoming)
                    } catch (e: IOException) {
                        onError(e)
                    }
                }, "cf-access-tunnel-accept").apply { isDaemon = true }.start()
            }

            override fun onMessage(webSocket: WebSocket, bytes: ByteString) {
                incoming.put(bytes.toByteArray())
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                incoming.put(closedMarker)
                onError(t)
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                incoming.put(closedMarker)
            }
        }

        webSocket = client.newWebSocket(request, listener)
    }

    private fun pumpSocketToWebSocket(socket: Socket, ws: WebSocket) {
        Thread({
            try {
                val input = socket.getInputStream()
                val buf = ByteArray(4096)
                while (true) {
                    val n = input.read(buf)
                    if (n < 0) break
                    ws.send(buf.copyOf(n).toByteString())
                }
            } catch (_: IOException) {
                // socket ferme, fin normale de session
            } finally {
                ws.close(1000, "session terminee")
            }
        }, "cf-access-tunnel-socket-to-ws").apply { isDaemon = true }.start()
    }

    private fun pumpWebSocketToSocket(socket: Socket, incoming: LinkedBlockingQueue<ByteArray>) {
        try {
            val output = socket.getOutputStream()
            while (true) {
                val data = incoming.take()
                if (data === closedMarker) break
                output.write(data)
                output.flush()
            }
        } catch (_: IOException) {
        } finally {
            socket.close()
        }
    }

    fun stop() {
        webSocket?.close(1000, "fin de session")
        webSocket = null
        // Debloque le thread bloque sur incoming.take() (sinon il fuit
        // jusqu'a la fin du process, meme en daemon).
        incoming?.put(closedMarker)
        incoming = null
        try { clientSocket?.close() } catch (_: IOException) {}
        clientSocket = null
        try { serverSocket?.close() } catch (_: IOException) {}
        serverSocket = null
    }
}
