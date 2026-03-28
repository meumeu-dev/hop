package dev.meumeu.hop.network

import android.util.Log
import dev.meumeu.hop.crypto.HopCrypto
import com.google.gson.Gson
import java.net.DatagramPacket
import java.net.DatagramSocket
import java.net.InetAddress
import java.net.Socket

object LanPairing {

    private const val TAG = "HOP-LAN"
    private const val UDP_PORT = 19876
    private const val TCP_PORT = 19877
    private const val BROADCAST_PREFIX = "HOP_PAIR:"
    private const val MAX_PACKET = 4096

    private val gson = Gson()

    /**
     * Listen for LAN broadcasts, decrypt with code, send response via TCP.
     * Returns the server's PairData on success.
     */
    fun joinLAN(code: String, myData: PairData, timeoutMs: Long = 120_000): PairData {
        Log.d(TAG, "Listening for LAN broadcasts on UDP $UDP_PORT...")
        val socket = DatagramSocket(UDP_PORT)
        socket.broadcast = true
        socket.soTimeout = 2000

        val deadline = System.currentTimeMillis() + timeoutMs
        try {
            while (System.currentTimeMillis() < deadline) {
                val buf = ByteArray(MAX_PACKET)
                val packet = DatagramPacket(buf, buf.size)
                try {
                    socket.receive(packet)
                } catch (_: java.net.SocketTimeoutException) {
                    continue
                }

                val data = String(packet.data, 0, packet.length)
                if (!data.startsWith(BROADCAST_PREFIX)) continue

                val encrypted = data.removePrefix(BROADCAST_PREFIX)
                Log.d(TAG, "Received broadcast from ${packet.address.hostAddress}")

                val decrypted = try {
                    HopCrypto.decrypt(encrypted, code)
                } catch (e: Exception) {
                    Log.d(TAG, "Decrypt failed (wrong code or not ours): ${e.message}")
                    continue
                }

                val serverData = gson.fromJson(String(decrypted), PairData::class.java)
                Log.d(TAG, "Found: ${serverData.hostname} (${serverData.ip})")

                // Send our response via TCP
                val serverIP = packet.address.hostAddress!!
                Log.d(TAG, "Sending response via TCP to $serverIP:$TCP_PORT")
                val responseJson = gson.toJson(myData).toByteArray()
                val responseEncrypted = HopCrypto.encrypt(responseJson, code)

                val tcp = Socket(serverIP, TCP_PORT)
                tcp.getOutputStream().write(responseEncrypted.toByteArray())
                tcp.close()

                Log.d(TAG, "LAN pairing complete with ${serverData.hostname}")
                return serverData
            }
        } finally {
            socket.close()
        }
        throw IllegalStateException("Timeout: aucun broadcast LAN detecte")
    }

    /**
     * Broadcast our data via UDP and wait for TCP response.
     * This is the "server" side of LAN pairing.
     */
    fun hostLAN(code: String, myData: PairData, timeoutMs: Long = 120_000): PairData {
        Log.d(TAG, "Starting LAN server: broadcasting on UDP $UDP_PORT, listening on TCP $TCP_PORT")

        val jsonData = gson.toJson(myData).toByteArray()
        val encrypted = HopCrypto.encrypt(jsonData, code)
        val broadcastData = (BROADCAST_PREFIX + encrypted).toByteArray()

        val tcpServer = java.net.ServerSocket(TCP_PORT)
        tcpServer.soTimeout = 2000

        val udpSocket = DatagramSocket()
        udpSocket.broadcast = true

        val deadline = System.currentTimeMillis() + timeoutMs
        try {
            while (System.currentTimeMillis() < deadline) {
                // Broadcast
                try {
                    val packet = DatagramPacket(
                        broadcastData, broadcastData.size,
                        InetAddress.getByName("255.255.255.255"), UDP_PORT
                    )
                    udpSocket.send(packet)
                    Log.d(TAG, "Broadcast sent")
                } catch (e: Exception) {
                    Log.w(TAG, "Broadcast error: ${e.message}")
                }

                // Check for TCP response
                try {
                    val client = tcpServer.accept()
                    val responseBytes = client.getInputStream().readBytes()
                    client.close()

                    val decrypted = HopCrypto.decrypt(String(responseBytes), code)
                    val clientData = gson.fromJson(String(decrypted), PairData::class.java)
                    Log.d(TAG, "LAN response from ${clientData.hostname}")
                    return clientData
                } catch (_: java.net.SocketTimeoutException) {
                    continue
                }
            }
        } finally {
            udpSocket.close()
            tcpServer.close()
        }
        throw IllegalStateException("Timeout: aucune reponse LAN recue")
    }
}
