package dev.meumeu.hop.crypto

import org.bouncycastle.crypto.generators.Argon2BytesGenerator
import org.bouncycastle.crypto.params.Argon2Parameters
import java.security.SecureRandom
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

object HopCrypto {

    private const val SALT_SIZE = 16
    private const val GCM_NONCE_SIZE = 12
    private const val GCM_TAG_BITS = 128
    private const val KEY_SIZE = 32

    // Argon2id params matching Go side: 3 iterations, 64MB, 1 thread
    private fun deriveKey(code: String, salt: ByteArray): ByteArray {
        val params = Argon2Parameters.Builder(Argon2Parameters.ARGON2_id)
            .withSalt(salt)
            .withIterations(3)
            .withMemoryAsKB(64 * 1024)
            .withParallelism(1)
            .build()

        val gen = Argon2BytesGenerator()
        gen.init(params)

        val key = ByteArray(KEY_SIZE)
        gen.generateBytes(code.toByteArray(Charsets.UTF_8), key)
        return key
    }

    fun encrypt(data: ByteArray, code: String): String {
        val random = SecureRandom()

        val salt = ByteArray(SALT_SIZE)
        random.nextBytes(salt)

        val key = deriveKey(code, salt)

        val nonce = ByteArray(GCM_NONCE_SIZE)
        random.nextBytes(nonce)

        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, SecretKeySpec(key, "AES"), GCMParameterSpec(GCM_TAG_BITS, nonce))
        val ciphertext = cipher.doFinal(data)

        // Output: salt || nonce || ciphertext (matches Go: salt || nonce || ciphertext+tag)
        val result = salt + nonce + ciphertext
        return Base64.getEncoder().encodeToString(result)
    }

    fun decrypt(encoded: String, code: String): ByteArray {
        val raw = Base64.getDecoder().decode(encoded)
        require(raw.size > SALT_SIZE + GCM_NONCE_SIZE) { "data too short" }

        val salt = raw.sliceArray(0 until SALT_SIZE)
        val nonce = raw.sliceArray(SALT_SIZE until SALT_SIZE + GCM_NONCE_SIZE)
        val ciphertext = raw.sliceArray(SALT_SIZE + GCM_NONCE_SIZE until raw.size)

        val key = deriveKey(code, salt)

        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, SecretKeySpec(key, "AES"), GCMParameterSpec(GCM_TAG_BITS, nonce))
        return cipher.doFinal(ciphertext)
    }

    fun generateCode(): String {
        val charset = "abcdefghijklmnopqrstuvwxyz0123456789"
        val random = SecureRandom()
        return (1..8).map { charset[random.nextInt(charset.length)] }.joinToString("")
    }
}
