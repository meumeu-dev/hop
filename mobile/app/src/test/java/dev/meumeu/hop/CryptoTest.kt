package dev.meumeu.hop

import org.bouncycastle.crypto.generators.Argon2BytesGenerator
import org.bouncycastle.crypto.params.Argon2Parameters
import org.bouncycastle.jce.provider.BouncyCastleProvider
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test
import java.security.Security
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

/**
 * Test that Kotlin decrypt matches Go encrypt for pairing data.
 * Uses real data captured from a live hop pair session.
 */
class CryptoTest {

    @Before
    fun setup() {
        Security.removeProvider("BC")
        Security.addProvider(BouncyCastleProvider())
    }

    // Same constants as HopCrypto.kt
    private val SALT_SIZE = 16
    private val GCM_NONCE_SIZE = 12
    private val GCM_TAG_BITS = 128
    private val KEY_SIZE = 32

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

    private fun decrypt(encoded: String, code: String): ByteArray {
        val raw = Base64.getDecoder().decode(encoded)
        assertTrue("Data too short: ${raw.size}", raw.size > SALT_SIZE + GCM_NONCE_SIZE)

        val salt = raw.sliceArray(0 until SALT_SIZE)
        val nonce = raw.sliceArray(SALT_SIZE until SALT_SIZE + GCM_NONCE_SIZE)
        val ciphertext = raw.sliceArray(SALT_SIZE + GCM_NONCE_SIZE until raw.size)

        println("salt (${salt.size}): ${salt.joinToString("") { "%02x".format(it) }}")
        println("nonce (${nonce.size}): ${nonce.joinToString("") { "%02x".format(it) }}")
        println("ciphertext (${ciphertext.size} bytes)")

        val key = deriveKey(code, salt)
        println("derived key: ${key.joinToString("") { "%02x".format(it) }}")

        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, SecretKeySpec(key, "AES"), GCMParameterSpec(GCM_TAG_BITS, nonce))
        return cipher.doFinal(ciphertext)
    }

    @Test
    fun testDecryptRealPairingData() {
        val code = "ynkcp43g"
        val encrypted = "f4Smc95EFadvh+BezJsEa31HL1qU7sj6bAwZsz2hfSwdCAJmN5tL4fiFcij0OM54mkYHmRWl725he/SHUC2nPWZPjjzt4xbiEyuriDcYD9lHuOIk2JqeD6ap9dGEsXMN0h3G9TA/7qPh1OIv5XBwXE2n3kNZuE3b8X8hBd9FH9TkFNby91kdfwFeBdTw+X1CMvnK4esU4G38GflbDNAb1LqPAs41XGvulijDJmMXqZM6gg+akuk9TzT0gZJrl93g8OiUAzvlcYg5xL3cLLBiYQSEGhdw2d6fnxTuuHQzwXqlyjbqhKk7z/SjGGgQmXYnqZJGtmFhNG3sacvOEyuOx45Q2RpMH6kmO1AjzJyIaNZV1+Wzyi4trhKAxjqxzBz3ekn1BrKfzdNeb69T2LGz5rJig797LPUY3MFv8vBqpHQyYzpreHHSkr1jI7BOuMRr/bIjzm6k6Ki7qAhM5G4hCj2gtGgVw2rABUiH3TekXhbjWhbdWpZXLvqtEi8Qlkaal56GeBPm3G0PXovR/uwKMvNHVmeCIB2fi15kRjG+n/u3IYo="

        println("Testing decrypt with code='$code', encrypted length=${encrypted.length}")

        val decrypted = decrypt(encrypted, code)
        val json = String(decrypted, Charsets.UTF_8)
        println("Decrypted JSON: $json")

        assertTrue("Should contain hostname", json.contains("hostname"))
        assertTrue("Should contain PC-Freelux", json.contains("PC-Freelux"))
    }

    @Test
    fun testEncryptDecryptRoundTrip() {
        val code = "testcode"
        val data = """{"hostname":"test","user":"hop","public_key":"ssh-ed25519 AAAA"}""".toByteArray()

        // Encrypt
        val salt = ByteArray(SALT_SIZE)
        java.security.SecureRandom().nextBytes(salt)
        val key = deriveKey(code, salt)
        val nonce = ByteArray(GCM_NONCE_SIZE)
        java.security.SecureRandom().nextBytes(nonce)

        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, SecretKeySpec(key, "AES"), GCMParameterSpec(GCM_TAG_BITS, nonce))
        val ciphertext = cipher.doFinal(data)

        val result = salt + nonce + ciphertext
        val encoded = Base64.getEncoder().encodeToString(result)

        // Decrypt
        val decrypted = decrypt(encoded, code)
        assertEquals(String(data), String(decrypted))
        println("Round-trip OK!")
    }
}
