package dev.cueseek.core.data

import androidx.test.ext.junit.runners.AndroidJUnit4
import dev.cueseek.core.data.internal.TokenCipher
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Test
import org.junit.runner.RunWith

/**
 * The Keystore, on real hardware.
 *
 * None of this can be checked on the JVM — there is no AndroidKeyStore provider — and a
 * mock of it would only assert that the mock behaves as imagined. On this device the key
 * is hardware-backed, so these tests are also the only evidence that the chosen
 * [android.security.keystore.KeyGenParameterSpec] is one the secure element accepts.
 */
@RunWith(AndroidJUnit4::class)
class TokenCipherTest {

    private val alias = "cueseek.test.${System.nanoTime()}"
    private val cipher = TokenCipher(alias)

    private val token = "csk_jsT8Bn6sLXCp1JXu6OtHmD2n6crjjjF7nw9nxVSQvHU"

    @After
    fun tearDown() = cipher.destroyKey()

    @Test
    fun sealed_token_round_trips() {
        assertEquals(token, cipher.unseal(cipher.seal(token)))
    }

    @Test
    fun sealed_value_does_not_contain_the_token() {
        val sealed = cipher.seal(token)

        assertNotEquals(token, sealed)
        assertFalse(sealed, sealed.contains(token))
        // The token's distinctive prefix must not survive either: a value that merely
        // looks encoded is exactly what makes a leak hard to spot in a file dump.
        assertFalse(sealed, sealed.contains("csk_"))
    }

    @Test
    fun the_same_token_seals_differently_every_time() {
        // GCM with a reused IV is catastrophic, and setRandomizedEncryptionRequired is the
        // setting that prevents it. If someone removes it, this is what notices.
        assertNotEquals(cipher.seal(token), cipher.seal(token))
    }

    @Test
    fun a_tampered_value_does_not_decrypt() {
        val sealed = cipher.seal(token)
        val tampered = sealed.dropLast(4) + if (sealed.endsWith("A")) "BBBB" else "AAAA"

        // GCM authenticates; a modified ciphertext must fail rather than yield plausible
        // rubbish that would then be sent to the agent as a bearer token.
        assertNull(cipher.unseal(tampered))
    }

    @Test
    fun garbage_input_is_not_an_exception() {
        assertNull(cipher.unseal("this is not base64 at all !!!"))
        assertNull(cipher.unseal(""))
        assertNull(cipher.unseal("AAAA"))
    }

    @Test
    fun a_value_sealed_with_a_dropped_key_is_unrecoverable() {
        val sealed = cipher.seal(token)
        cipher.destroyKey()

        // Losing the key is the same event as the user clearing app data or restoring onto
        // different hardware. It must present as "not paired", not as a crash: a fresh key
        // is generated on demand and cannot open the old ciphertext.
        assertNull(cipher.unseal(sealed))
    }

    @Test
    fun the_key_survives_being_looked_up_again() {
        val sealed = cipher.seal(token)

        // A second instance must find the existing key rather than generate a new one -
        // otherwise every process start would silently invalidate the stored token.
        assertEquals(token, TokenCipher(alias).unseal(sealed))
    }
}
