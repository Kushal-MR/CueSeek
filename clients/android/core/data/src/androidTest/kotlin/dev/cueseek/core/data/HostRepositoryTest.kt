package dev.cueseek.core.data

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import androidx.datastore.preferences.core.Preferences
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import dev.cueseek.core.data.internal.TokenCipher
import dev.cueseek.core.model.AgentAddress
import dev.cueseek.core.model.DeviceId
import dev.cueseek.core.model.DeviceToken
import dev.cueseek.core.model.HostId
import dev.cueseek.core.model.PairedHost
import dev.cueseek.core.model.Scope
import java.io.File
import java.time.Instant
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * The host registry against a real DataStore and a real Keystore.
 *
 * The assertion that matters most is [the_token_is_never_written_in_plaintext]: everything
 * else here is bookkeeping that a JVM fake would test just as well, but whether the bytes
 * on disk contain the credential is a property of the actual storage stack.
 */
@RunWith(AndroidJUnit4::class)
class HostRepositoryTest {

    private val context = InstrumentationRegistry.getInstrumentation().targetContext
    private val alias = "cueseek.test.${System.nanoTime()}"
    private val cipher = TokenCipher(alias)

    private lateinit var file: File
    private var scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private lateinit var store: DataStore<Preferences>
    private lateinit var repository: HostRepository

    private val token = DeviceToken("csk_jsT8Bn6sLXCp1JXu6OtHmD2n6crjjjF7nw9nxVSQvHU")

    private val host = PairedHost(
        hostId = HostId("664917f8b739290c57d971481accef0e"),
        address = AgentAddress("100.92.18.125", 7777),
        hostname = "kushal-HP-paviliong6",
        agentVersion = "m1.8-listenretry",
        apiVersion = "0.1.0",
        deviceId = DeviceId("217f2f3dbf991996"),
        deviceName = "Pixel 8",
        scopes = setOf(Scope.Read, Scope.ServiceControl),
        pairedAt = Instant.parse("2026-08-10T05:38:14Z"),
    )

    @Before
    fun setUp() {
        file = File(context.filesDir, "cueseek-test-${System.nanoTime()}.preferences_pb")
        store = PreferenceDataStoreFactory.create(scope = scope) { file }
        repository = HostRepository(store, cipher)
    }

    @After
    fun tearDown() {
        scope.cancel()
        file.delete()
        cipher.destroyKey()
    }

    /** Closes this DataStore and opens a new one over the same file, as a restart would. */
    private fun reopen(): HostRepository {
        scope.cancel()
        scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
        store = PreferenceDataStoreFactory.create(scope = scope) { file }
        return HostRepository(store, cipher)
    }

    @Test
    fun a_paired_host_and_its_token_are_stored() = runBlocking {
        repository.save(host, token)

        assertEquals(listOf(host), repository.hosts.first())
        assertEquals(token, repository.token(host.hostId))
        assertEquals(host, repository.selectedHost.first())
    }

    @Test
    fun the_token_is_never_written_in_plaintext() = runBlocking {
        repository.save(host, token)

        // ISO-8859-1 keeps every byte addressable as a character, so a token written in
        // any encoding this file could plausibly use would still show up.
        val raw = String(file.readBytes(), Charsets.ISO_8859_1)

        assertTrue("the store should not be empty", raw.isNotEmpty())
        assertFalse("the token is on disk in the clear", raw.contains(token.value))
        assertFalse("even the token prefix must not appear", raw.contains("csk_"))
        // The non-secret record is expected to be readable - that is the point of keeping
        // it out of the sealed value.
        assertTrue("host record should be stored in the clear", raw.contains(host.hostname))
    }

    @Test
    fun state_survives_a_restart() = runBlocking {
        repository.save(host, token)

        val restarted = reopen()

        assertEquals(listOf(host), restarted.hosts.first())
        // The in-memory cache is gone with the old instance, so this necessarily went
        // through the Keystore again.
        assertEquals(token, restarted.token(host.hostId))
    }

    @Test
    fun a_second_host_is_just_another_record() = runBlocking {
        val nas = host.copy(
            hostId = HostId("aaaa1111bbbb2222"),
            address = AgentAddress("100.64.0.9", 8080),
            hostname = "nas",
        )

        repository.save(host, token)
        repository.save(nas, DeviceToken("csk_second_token_value_here_0000000000000000"))

        assertEquals(2, repository.hosts.first().size)
        assertEquals(token, repository.token(host.hostId))
        // Selection follows the most recent pairing; the earlier host is untouched.
        assertEquals(nas, repository.selectedHost.first())

        repository.select(host.hostId)
        assertEquals(host, repository.selectedHost.first())
    }

    @Test
    fun re_pairing_replaces_rather_than_duplicates() = runBlocking {
        repository.save(host, token)
        val fresh = DeviceToken("csk_a_completely_new_token_0000000000000000000")

        repository.save(host.copy(hostname = "renamed"), fresh)

        assertEquals(1, repository.hosts.first().size)
        assertEquals("renamed", repository.hosts.first().single().hostname)
        assertEquals(fresh, repository.token(host.hostId))
    }

    @Test
    fun forgetting_a_host_removes_its_credential() = runBlocking {
        repository.save(host, token)

        repository.forget(host.hostId)

        assertTrue(repository.hosts.first().isEmpty())
        assertNull(repository.token(host.hostId))
        assertNull(repository.selectedHost.first())

        val raw = String(file.readBytes(), Charsets.ISO_8859_1)
        assertFalse("the sealed token should be gone too", raw.contains("token."))
    }

    @Test
    fun an_address_change_keeps_the_identity_and_the_token() = runBlocking {
        repository.save(host, token)

        repository.updateAddress(host.hostId, AgentAddress("100.92.18.200", 7777))

        val updated = repository.hosts.first().single()
        // ADR-0008: the address is where a host was reachable; the id is what it is. A
        // DHCP lease change must not look like a new server.
        assertEquals(host.hostId, updated.hostId)
        assertEquals("100.92.18.200", updated.address.host)
        assertEquals(token, repository.token(host.hostId))
    }

    @Test
    fun a_token_that_cannot_be_decrypted_drops_the_host() = runBlocking {
        repository.save(host, token)

        // Exactly what the user sees after clearing app data or restoring a backup onto
        // different hardware: the ciphertext is still there and the key is not.
        cipher.destroyKey()
        val restarted = reopen()

        assertNull(restarted.token(host.hostId))
        assertTrue(
            "the app must present itself as unpaired, not as paired-but-broken",
            restarted.hosts.first().isEmpty(),
        )
    }

    @Test
    fun the_cached_token_is_available_to_the_interceptor_after_a_read() = runBlocking {
        repository.save(host, token)
        val restarted = reopen()

        // The interceptor cannot suspend, so the cache must be warm before it asks.
        assertNull(restarted.cachedToken(host.hostId))
        restarted.token(host.hostId)
        assertNotNull(restarted.cachedToken(host.hostId))
    }
}
