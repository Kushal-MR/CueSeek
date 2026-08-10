package dev.cueseek.core.data.internal

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.security.keystore.KeyPermanentlyInvalidatedException
import android.util.Base64
import java.security.GeneralSecurityException
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * Seals the device token with a key that never leaves the Android Keystore.
 *
 * ADR-0006 originally named `EncryptedSharedPreferences`; the Jetpack Security library it
 * comes from is unmaintained, so this is the forty lines that library used to own
 * (ADR-0013, and ADR-0006 Amendment 2). The property being preserved is unchanged: the
 * token is never at rest in plaintext, and the key protecting it is held by
 * hardware-backed storage rather than by this application.
 *
 * **The key requires no user authentication and is not invalidated by biometric
 * enrollment.** That is a deliberate posture, not an oversight. ADR-0001 already scopes
 * the threat model to a device that can reach the tailnet, and CueSeek's primary
 * interaction is a two-second glance at whether anything is broken — a keyguard prompt
 * before rendering service health would make the console unusable for its actual purpose.
 * Step-up authentication belongs on individual destructive actions, gated on
 * [dev.cueseek.core.model.ActionRisk], which is an M3 concern.
 *
 * What the key still buys: the token cannot be read off the device by anything that gets
 * at the app's files, it cannot be exported, and on hardware-backed devices it cannot be
 * extracted from the secure element at all.
 */
internal class TokenCipher(
    private val keyAlias: String = DEFAULT_KEY_ALIAS,
) {

    /** Returns base64 of `iv || ciphertext`, or throws [GeneralSecurityException]. */
    fun seal(plaintext: String): String {
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.ENCRYPT_MODE, key())

        val iv = cipher.iv
        require(iv.size == IV_LENGTH) { "unexpected GCM IV length ${iv.size}" }

        val ciphertext = cipher.doFinal(plaintext.toByteArray(Charsets.UTF_8))
        return Base64.encodeToString(iv + ciphertext, Base64.NO_WRAP)
    }

    /**
     * Reverses [seal], or returns `null` if the sealed value cannot be recovered.
     *
     * Null is a real outcome rather than an error path to be logged and ignored. The key
     * can genuinely disappear — the user clears app data, restores a backup onto different
     * hardware, or the Keystore is reset — and every one of those means the same thing to
     * the user: this device is no longer paired and must pair again. Surfacing it as an
     * exception would push that decision to a caller that has no better answer.
     */
    fun unseal(sealed: String): String? = try {
        val bytes = Base64.decode(sealed, Base64.NO_WRAP)
        if (bytes.size <= IV_LENGTH) return null

        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(
            Cipher.DECRYPT_MODE,
            key(),
            GCMParameterSpec(TAG_LENGTH_BITS, bytes, 0, IV_LENGTH),
        )
        String(
            cipher.doFinal(bytes, IV_LENGTH, bytes.size - IV_LENGTH),
            Charsets.UTF_8,
        )
    } catch (_: KeyPermanentlyInvalidatedException) {
        // Not reachable under the current key policy, but the policy is one line away from
        // changing and this is the branch a future change would silently break.
        null
    } catch (_: GeneralSecurityException) {
        // Includes AEADBadTagException: the ciphertext was truncated or tampered with.
        null
    } catch (_: IllegalArgumentException) {
        // Not valid base64. Something other than this class wrote the value.
        null
    }

    /** Drops the key, making every previously sealed value permanently unrecoverable. */
    fun destroyKey() {
        keyStore().deleteEntry(keyAlias)
    }

    private fun key(): SecretKey {
        val existing = keyStore().getEntry(keyAlias, null) as? KeyStore.SecretKeyEntry
        return existing?.secretKey ?: generateKey()
    }

    private fun generateKey(): SecretKey {
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, PROVIDER)
        generator.init(
            KeyGenParameterSpec.Builder(
                keyAlias,
                KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
            )
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(KEY_SIZE_BITS)
                // Forces a fresh IV per operation and forbids callers supplying one, which
                // is what keeps GCM from being catastrophically misused by reuse.
                .setRandomizedEncryptionRequired(true)
                .build()
        )
        return generator.generateKey()
    }

    private fun keyStore(): KeyStore =
        KeyStore.getInstance(PROVIDER).apply { load(null) }

    private companion object {
        const val PROVIDER = "AndroidKeyStore"
        const val TRANSFORMATION = "AES/GCM/NoPadding"
        const val DEFAULT_KEY_ALIAS = "cueseek.device-token.v1"
        const val KEY_SIZE_BITS = 256
        const val IV_LENGTH = 12
        const val TAG_LENGTH_BITS = 128
    }
}
