package dev.meumeu.hop.unlock

import android.content.Context
import android.os.Build
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyPermanentlyInvalidatedException
import android.security.keystore.KeyProperties
import android.util.Base64
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * Coffre de la passphrase LUKS, scelle par l'empreinte.
 *
 * La passphrase est chiffree en AES-256-GCM par une cle generee DANS le
 * Keystore materiel du telephone (TEE/StrongBox) avec
 * `setUserAuthenticationRequired(true)` : la cle n'est pas exportable, meme
 * avec root, et le dechiffrement est refuse par le materiel tant qu'une
 * empreinte valide n'a pas ete presentee pour CETTE operation precise
 * (CryptoObject lie au Cipher).
 *
 * `setInvalidatedByBiometricEnrollment(true)` : si une nouvelle empreinte est
 * enregistree sur le telephone, la cle est detruite automatiquement — la
 * passphrase devient illisible et devra etre re-saisie. C'est voulu (empeche
 * qu'un tiers ajoute son doigt pour ouvrir le coffre).
 */
object UnlockVault {
    private const val PREFS = "hop_unlock"
    private const val TRANSFORMATION = "AES/GCM/NoPadding"

    // Une cle Keystore et un blob distincts par machine : effacer la
    // passphrase d'une machine ne touche pas les autres.
    private fun alias(targetId: String) = "hop_unlock_vault_$targetId"
    private fun keyBlob(targetId: String) = "passphrase_blob_$targetId"
    private fun keyIv(targetId: String) = "passphrase_iv_$targetId"

    private fun prefs(context: Context) =
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)

    fun hasPassphrase(context: Context, targetId: String): Boolean =
        prefs(context).contains(keyBlob(targetId)) && prefs(context).contains(keyIv(targetId))

    fun clear(context: Context, targetId: String) {
        prefs(context).edit().remove(keyBlob(targetId)).remove(keyIv(targetId)).apply()
        try {
            KeyStore.getInstance("AndroidKeyStore").apply { load(null) }.deleteEntry(alias(targetId))
        } catch (_: Exception) {
        }
    }

    private fun loadKey(targetId: String): SecretKey? {
        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        return ks.getKey(alias(targetId), null) as? SecretKey
    }

    private fun createKey(targetId: String): SecretKey {
        val kg = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
        val builder = KeyGenParameterSpec.Builder(
            alias(targetId),
            KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT
        )
            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
            .setKeySize(256)
            .setUserAuthenticationRequired(true)
            .setInvalidatedByBiometricEnrollment(true)

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            // 0 = authentification exigee pour CHAQUE operation (via CryptoObject)
            builder.setUserAuthenticationParameters(0, KeyProperties.AUTH_BIOMETRIC_STRONG)
        } else {
            @Suppress("DEPRECATION")
            builder.setUserAuthenticationValidityDurationSeconds(-1)
        }

        kg.init(builder.build())
        return kg.generateKey()
    }

    /**
     * Cipher pret pour l'enregistrement, a passer a BiometricPrompt.CryptoObject.
     * Recree la cle si elle a ete invalidee (nouvelle empreinte enregistree).
     */
    fun encryptCipher(targetId: String): Cipher {
        val key = try {
            loadKey(targetId) ?: createKey(targetId)
        } catch (_: Exception) {
            createKey(targetId)
        }
        return Cipher.getInstance(TRANSFORMATION).apply { init(Cipher.ENCRYPT_MODE, key) }
    }

    /**
     * Cipher pret pour la lecture, a passer a BiometricPrompt.CryptoObject.
     * Retourne null si aucune passphrase enregistree, ou si la cle a ete
     * invalidee (dans ce cas le coffre est vide et il faut re-saisir).
     */
    fun decryptCipher(context: Context, targetId: String): Cipher? {
        if (!hasPassphrase(context, targetId)) return null
        val ivB64 = prefs(context).getString(keyIv(targetId), null) ?: return null
        val iv = Base64.decode(ivB64, Base64.NO_WRAP)
        val key = try {
            loadKey(targetId) ?: return null
        } catch (_: Exception) {
            return null
        }
        return try {
            Cipher.getInstance(TRANSFORMATION).apply {
                init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(128, iv))
            }
        } catch (_: KeyPermanentlyInvalidatedException) {
            clear(context, targetId) // nouvelle empreinte : coffre inutilisable
            null
        } catch (_: Exception) {
            null
        }
    }

    /** Appele APRES succes biometrique, avec le cipher issu du CryptoObject. */
    fun store(context: Context, targetId: String, cipher: Cipher, passphrase: String) {
        val blob = cipher.doFinal(passphrase.toByteArray(Charsets.UTF_8))
        prefs(context).edit()
            .putString(keyBlob(targetId), Base64.encodeToString(blob, Base64.NO_WRAP))
            .putString(keyIv(targetId), Base64.encodeToString(cipher.iv, Base64.NO_WRAP))
            .apply()
    }

    /** Appele APRES succes biometrique, avec le cipher issu du CryptoObject. */
    fun retrieve(context: Context, targetId: String, cipher: Cipher): String? {
        val blobB64 = prefs(context).getString(keyBlob(targetId), null) ?: return null
        return try {
            String(cipher.doFinal(Base64.decode(blobB64, Base64.NO_WRAP)), Charsets.UTF_8)
        } catch (_: Exception) {
            null
        }
    }
}
