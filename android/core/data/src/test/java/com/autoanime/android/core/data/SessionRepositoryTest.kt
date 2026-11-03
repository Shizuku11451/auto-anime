package com.autoanime.android.core.data

import com.autoanime.android.core.model.AnimeDetail
import com.autoanime.android.core.model.AnimePage
import com.autoanime.android.core.model.AppError
import com.autoanime.android.core.model.LoginResult
import com.autoanime.android.core.model.SetupState
import com.autoanime.android.core.network.AnimeApi
import com.autoanime.android.core.network.ApiError
import com.autoanime.android.core.network.CookieStorage
import com.autoanime.android.core.network.ServerConnection
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import okhttp3.Cookie
import okhttp3.OkHttpClient
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class SessionRepositoryTest {
    private val dispatcher = StandardTestDispatcher()
    private val scope = TestScope(dispatcher)
    private val store = FakeConnectionStore()
    private val apis = mutableListOf<FakeApi>()
    private var nextApi: () -> FakeApi = { FakeApi() }
    private val repository = SessionRepository(
        cookieStorage = { MemoryCookieStorage() },
        connectionStore = store,
        apiFactory = { _, _ -> nextApi().also { apis += it } },
        imageLoaderFactory = { error("covers are not loaded in unit tests") },
        scope = scope.backgroundScope,
        io = dispatcher,
    )
    private val server = ServerConnection.parse("https://nas.example/", false)

    @Test fun restoreReusesStoredSessionAndNeverLogsInWithoutPassword() = scope.runTest {
        store.saved = SavedConnection("https://nas.example/", false, "alice")
        repository.restore()
        runCurrent()
        val api = apis.single()
        api.setup.complete(SetupState(false))
        runCurrent()
        assertEquals(0, api.loginCalls)
        assertEquals(1, api.probeCalls)
        assertTrue(repository.state.value is SessionState.Connected)
    }

    @Test fun duplicateUnauthorizedReportsPublishOneSignedOutTransition() = scope.runTest {
        val session = connect()
        repository.onUnauthorized(session)
        val first = repository.state.value
        repository.onUnauthorized(session)
        assertSame(first, repository.state.value)
        assertEquals(AppError.SessionExpired, (first as SessionState.SignedOut).error)
    }

    @Test fun lateConnectResultCannotReplaceNewerConnection() = scope.runTest {
        repository.connect(server, "", "")
        runCurrent()
        val old = apis.last()
        repository.connect(ServerConnection.parse("https://other.example/", false), "", "")
        runCurrent()
        apis.last().setup.complete(SetupState(false))
        runCurrent()
        old.setup.complete(SetupState(false))
        runCurrent()
        val session = (repository.state.value as SessionState.Connected).session
        assertEquals("https://other.example/", session.connection.base.toString())
    }

    @Test fun logoutThenReconnectDoesNotClearNewSavedConnection() = scope.runTest {
        store.saved = SavedConnection(server.base.toString(), false, "")
        val delayed = CompletableDeferred<Unit>()
        store.saveGate = delayed
        repository.connect(server, "", "password")
        runCurrent()
        apis.last().setup.complete(SetupState(false))
        runCurrent()
        assertTrue(store.savesStarted)
        repository.logout()
        runCurrent()
        repository.restore()
        runCurrent()
        assertTrue("logout must not restore the old target", repository.state.value is SessionState.SignedOut)
        store.saveGate = null
        repository.connect(ServerConnection.parse("https://new.example/", false), "", "")
        runCurrent()
        apis.last().setup.complete(SetupState(false))
        runCurrent()
        delayed.complete(Unit)
        runCurrent()
        assertEquals("https://new.example/", store.saved?.url)
    }

    @Test fun logoutClearsSavedConnection() = scope.runTest {
        connect()
        repository.logout()
        runCurrent()
        assertNull(store.saved)
        assertTrue(repository.state.value is SessionState.SignedOut)
    }

    @Test fun unauthorizedConnectShowsExpiredSession() = scope.runTest {
        nextApi = { FakeApi(probeError = ApiError(401, "Session expired")) }
        repository.connect(server, "alice", "")
        runCurrent()
        apis.last().setup.complete(SetupState(false))
        runCurrent()
        assertEquals(AppError.SessionExpired, (repository.state.value as SessionState.SignedOut).error)
    }

    @Test fun setupRequiredIsReported() = scope.runTest {
        repository.connect(server, "", "")
        runCurrent()
        apis.last().setup.complete(SetupState(true))
        runCurrent()
        assertEquals(AppError.SetupRequired, (repository.state.value as SessionState.SignedOut).error)
    }

    private fun TestScope.connect(): Session {
        repository.connect(server, "", "")
        runCurrent()
        apis.last().setup.complete(SetupState(false))
        runCurrent()
        return (repository.state.value as SessionState.Connected).session
    }
}

private class FakeApi(private val probeError: Exception? = null) :
    AnimeApi(OkHttpClient(), ServerConnection.parse("https://nas.example/", false)) {
    val setup = CompletableDeferred<SetupState>()
    var loginCalls = 0
    var probeCalls = 0
    override suspend fun setupStatus() = setup.await()
    override suspend fun login(user: String, password: String): LoginResult { loginCalls++; return LoginResult(true) }
    override suspend fun probeSession() { probeCalls++; probeError?.let { throw it } }
    override suspend fun page(before: Long?, day: Int?, today: String?, counts: Boolean): AnimePage = AnimePage(emptyList(), 0)
    override suspend fun detail(id: Long): AnimeDetail = AnimeDetail(id, "")
    override suspend fun logout() = Unit
}

private class FakeConnectionStore : ConnectionStore {
    var saved: SavedConnection? = null
    var saveGate: CompletableDeferred<Unit>? = null
    var savesStarted = false
    override suspend fun load() = saved
    override suspend fun save(connection: SavedConnection) {
        savesStarted = true
        saveGate?.await()
        saved = connection
    }
    override suspend fun clear() { saved = null }
}

private class MemoryCookieStorage : CookieStorage {
    private val values = mutableMapOf<String, List<Cookie>>()
    override fun load(origin: String) = Unit
    override fun save(origin: String, cookies: List<Cookie>) { values[origin] = cookies }
    override fun cookies(origin: String) = values[origin].orEmpty()
    override fun clear(origin: String) { values.remove(origin) }
}
