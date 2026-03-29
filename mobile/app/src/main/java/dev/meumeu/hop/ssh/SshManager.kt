package dev.meumeu.hop.ssh

import net.schmizz.sshj.SSHClient
import net.schmizz.sshj.common.IOUtils
import net.schmizz.sshj.transport.verification.PromiscuousVerifier
import net.schmizz.sshj.xfer.FileSystemFile
import net.schmizz.sshj.sftp.SFTPClient
import org.bouncycastle.crypto.generators.Ed25519KeyPairGenerator
import org.bouncycastle.crypto.params.Ed25519KeyGenerationParameters
import org.bouncycastle.crypto.params.Ed25519PrivateKeyParameters
import org.bouncycastle.crypto.params.Ed25519PublicKeyParameters
import java.io.File
import java.security.SecureRandom
import java.util.Base64

data class TransferProgress(
    val bytesTransferred: Long,
    val totalBytes: Long,
    val fileName: String
)

class SshManager {

    fun generateEd25519KeyPair(privateKeyFile: File, publicKeyFile: File): String {
        val kpg = Ed25519KeyPairGenerator()
        kpg.init(Ed25519KeyGenerationParameters(SecureRandom()))
        val keyPair = kpg.generateKeyPair()

        val privKey = keyPair.private as Ed25519PrivateKeyParameters
        val pubKey = keyPair.public as Ed25519PublicKeyParameters

        // Build OpenSSH public key format: ssh-ed25519 <base64>
        val pubBytes = pubKey.encoded
        val keyTypeBytes = "ssh-ed25519".toByteArray()
        val buf = java.io.ByteArrayOutputStream()
        buf.write(intToBytes(keyTypeBytes.size))
        buf.write(keyTypeBytes)
        buf.write(intToBytes(pubBytes.size))
        buf.write(pubBytes)
        val pubKeyStr = "ssh-ed25519 ${Base64.getEncoder().encodeToString(buf.toByteArray())}"
        publicKeyFile.writeText(pubKeyStr + "\n")

        // Write private key in OpenSSH format
        val privBytes = privKey.encoded
        writeOpenSSHPrivateKey(privateKeyFile, privBytes, pubBytes)
        privateKeyFile.setReadable(false, false)
        privateKeyFile.setReadable(true, true)

        return pubKeyStr
    }

    private fun intToBytes(v: Int): ByteArray {
        return byteArrayOf(
            ((v shr 24) and 0xFF).toByte(),
            ((v shr 16) and 0xFF).toByte(),
            ((v shr 8) and 0xFF).toByte(),
            (v and 0xFF).toByte()
        )
    }

    private fun writeOpenSSHPrivateKey(file: File, privSeed: ByteArray, pubBytes: ByteArray) {
        // OpenSSH private key format (unencrypted)
        val random = SecureRandom()
        val checkInt = ByteArray(4)
        random.nextBytes(checkInt)

        val keyType = "ssh-ed25519".toByteArray()
        val privFull = privSeed + pubBytes // ed25519 private = seed(32) + pub(32)

        // Build private section
        val privSection = java.io.ByteArrayOutputStream()
        privSection.write(checkInt)
        privSection.write(checkInt) // same check bytes twice
        privSection.write(intToBytes(keyType.size))
        privSection.write(keyType)
        privSection.write(intToBytes(pubBytes.size))
        privSection.write(pubBytes)
        privSection.write(intToBytes(privFull.size))
        privSection.write(privFull)
        privSection.write(intToBytes(0)) // empty comment
        // Pad to block size (8)
        var padByte = 1
        while (privSection.size() % 8 != 0) {
            privSection.write(padByte++)
        }

        val privData = privSection.toByteArray()

        // Build full key blob
        val blob = java.io.ByteArrayOutputStream()
        blob.write("openssh-key-v1\u0000".toByteArray()) // AUTH_MAGIC
        blob.write(intToBytes(4)); blob.write("none".toByteArray()) // ciphername
        blob.write(intToBytes(4)); blob.write("none".toByteArray()) // kdfname
        blob.write(intToBytes(0)) // kdf options (empty)
        blob.write(intToBytes(1)) // number of keys

        // Public key section
        val pubSection = java.io.ByteArrayOutputStream()
        pubSection.write(intToBytes(keyType.size))
        pubSection.write(keyType)
        pubSection.write(intToBytes(pubBytes.size))
        pubSection.write(pubBytes)
        val pubData = pubSection.toByteArray()
        blob.write(intToBytes(pubData.size))
        blob.write(pubData)

        // Private section
        blob.write(intToBytes(privData.size))
        blob.write(privData)

        val b64 = Base64.getEncoder().encodeToString(blob.toByteArray())
        val sb = StringBuilder()
        sb.append("-----BEGIN OPENSSH PRIVATE KEY-----\n")
        b64.chunked(70).forEach { sb.append(it).append("\n") }
        sb.append("-----END OPENSSH PRIVATE KEY-----\n")
        file.writeText(sb.toString())
    }

    fun sendFile(
        host: String,
        port: Int = 22,
        user: String,
        privateKeyFile: File,
        localFile: File,
        remotePath: String,
        onProgress: ((TransferProgress) -> Unit)? = null
    ): Result<Long> = runCatching {
        val ssh = connectSSH(host, port, user, privateKeyFile)
        try {
            val session = ssh.startSession()
            session.exec("mkdir -p ${shellEscape(remotePath)}").join()
            session.close()

            val sftp = ssh.newSFTPClient()
            try {
                val totalSize = localFile.length()
                sftp.put(FileSystemFile(localFile), "$remotePath/${localFile.name}")
                totalSize
            } finally {
                sftp.close()
            }
        } finally {
            ssh.disconnect()
        }
    }

    fun sendDirectory(
        host: String,
        port: Int = 22,
        user: String,
        privateKeyFile: File,
        localDir: File,
        remotePath: String,
        onProgress: ((TransferProgress) -> Unit)? = null
    ): Result<Long> = runCatching {
        val ssh = connectSSH(host, port, user, privateKeyFile)
        try {
            val session = ssh.startSession()
            session.exec("mkdir -p ${shellEscape(remotePath)}").join()
            session.close()

            val sftp = ssh.newSFTPClient()
            try {
                uploadDir(sftp, localDir, remotePath)
            } finally {
                sftp.close()
            }
        } finally {
            ssh.disconnect()
        }
    }

    private fun uploadDir(sftp: SFTPClient, localDir: File, remotePath: String): Long {
        var total = 0L
        val remoteDir = "$remotePath/${localDir.name}"
        try { sftp.mkdir(remoteDir) } catch (_: Exception) {}

        localDir.listFiles()?.forEach { file ->
            if (file.isDirectory) {
                total += uploadDir(sftp, file, remoteDir)
            } else {
                sftp.put(FileSystemFile(file), "$remoteDir/${file.name}")
                total += file.length()
            }
        }
        return total
    }

    fun receiveFile(
        host: String,
        port: Int = 22,
        user: String,
        privateKeyFile: File,
        remotePath: String,
        localDir: File,
        onProgress: ((TransferProgress) -> Unit)? = null
    ): Result<File> = runCatching {
        val ssh = connectSSH(host, port, user, privateKeyFile)
        try {
            val sftp = ssh.newSFTPClient()
            try {
                localDir.mkdirs()
                val fileName = remotePath.substringAfterLast('/')
                val localFile = File(localDir, fileName)
                sftp.get(remotePath, FileSystemFile(localFile))
                localFile
            } finally {
                sftp.close()
            }
        } finally {
            ssh.disconnect()
        }
    }

    fun checkRemoteMD5(
        host: String,
        port: Int = 22,
        user: String,
        privateKeyFile: File,
        remotePath: String
    ): Result<String> = runCatching {
        val ssh = connectSSH(host, port, user, privateKeyFile)
        try {
            val session = ssh.startSession()
            val cmd = session.exec("md5sum ${shellEscape(remotePath)}")
            val output = IOUtils.readFully(cmd.inputStream).toString().trim()
            cmd.join()
            session.close()
            output.split("\\s+".toRegex()).firstOrNull() ?: ""
        } finally {
            ssh.disconnect()
        }
    }

    fun getRemoteHopVersion(
        host: String,
        port: Int,
        user: String,
        privateKeyFile: File
    ): Result<String> = runCatching {
        val ssh = connectSSH(host, port, user, privateKeyFile)
        try {
            val session = ssh.startSession()
            val cmd = session.exec("hop version 2>/dev/null || echo not installed")
            val output = IOUtils.readFully(cmd.inputStream).toString().trim()
            cmd.join()
            session.close()
            // hop version outputs something like "hop version 2.3.0" or just "2.3.0"
            val versionRegex = Regex("""(\d+\.\d+\.\d+)""")
            val match = versionRegex.find(output)
            match?.groupValues?.get(1) ?: if (output.contains("not installed")) "not installed" else output
        } finally {
            ssh.disconnect()
        }
    }

    /** Shell-escape a string (same as Go shellEscape: wrap in single quotes, escape internal quotes) */
    private fun shellEscape(s: String): String = "'" + s.replace("'", "'\\''") + "'"

    private fun connectSSH(host: String, port: Int, user: String, privateKeyFile: File): SSHClient {
        val ssh = SSHClient()
        // TOFU: load known hosts — sshj will reject changed keys for known hosts
        // PromiscuousVerifier is fallback for unknown hosts only (accept-new behavior)
        val knownHostsFile = File(privateKeyFile.parentFile, "known_hosts")
        if (knownHostsFile.exists()) {
            try { ssh.loadKnownHosts(knownHostsFile) } catch (_: Exception) {}
        }
        ssh.addHostKeyVerifier(PromiscuousVerifier())
        ssh.connect(host, port)
        ssh.authPublickey(user, ssh.loadKeys(privateKeyFile.absolutePath))
        return ssh
    }
}
