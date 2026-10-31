package com.autoanime.android.feature

import androidx.arch.core.executor.testing.InstantTaskExecutorRule
import androidx.lifecycle.viewModelScope
import com.autoanime.android.SavedConnection
import com.autoanime.android.SessionConnection
import com.autoanime.android.core.*
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.*
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.*
import okhttp3.Cookie
import okhttp3.CookieJar
import okhttp3.HttpUrl
import okhttp3.OkHttpClient
import org.junit.After
import org.junit.Assert.*
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import java.util.concurrent.atomic.AtomicLong

@OptIn(ExperimentalCoroutinesApi::class)
class ClientViewModelTest {
    @get:Rule val instant = InstantTaskExecutorRule()
    private val main = StandardTestDispatcher()
    private val storage = MemoryStorage()
    private val app = FakeSessionConnection(storage)
    private lateinit var server: ServerConnection
    private lateinit var api: FakeApi
    private lateinit var model: ClientViewModel

    @Before fun setup() {
        Dispatchers.setMain(main)
        server = ServerConnection.parse("https://nas.example/", false)
        api = FakeApi()
        model = ClientViewModel({ _, _ -> api }, main)
    }
    @After fun tearDown() { model.clearForTest(); Dispatchers.resetMain() }

    @Test fun detailBWinsWhenACompletesLate() = runTest(main) {
        connect()
        api.nonCancellableDetail = true
        model.loadDetail(1)
        runCurrent()
        val gateA = api.details.getValue(1)
        model.loadDetail(2)
        runCurrent()
        val gateB = api.details.getValue(2)
        gateB.complete(AnimeDetail(2, "B")); runCurrent()
        gateA.complete(AnimeDetail(1, "A")); runCurrent()
        assertEquals(2L, model.state.value.detail?.id)
    }

    @Test fun duplicateUnauthorizedResponsesOnlyPublishOneLoginTransition() = runTest(main) {
        connect()
        val session = model.state.value.sessionCacheKey
        model.expireSession(app)
        runCurrent()
        val after401 = model.state.value.sessionCacheKey
        model.expireSession(app)
        runCurrent()
        assertTrue(after401 > session)
        assertEquals(after401, model.state.value.sessionCacheKey)
        assertTrue(model.state.value.requiresLogin)
    }

    @Test fun oldConnectionPageCannotReplaceNewConnection() = runTest(main) {
        model.connect(app, server, "", ""); main.scheduler.runCurrent()
        api.setupGate.complete(SetupState(false)); main.scheduler.runCurrent()
        val oldPage = api.pages.last()
        api = FakeApi()
        model.connect(app, ServerConnection.parse("https://other.example/", false), "", ""); main.scheduler.runCurrent()
        api.setupGate.complete(SetupState(false)); main.scheduler.runCurrent()
        val newPage = api.pages.last()
        newPage.complete(AnimePage(listOf(Anime(2, "new")), 0)); main.scheduler.runCurrent()
        oldPage.complete(AnimePage(listOf(Anime(1, "old")), 0)); main.scheduler.runCurrent()
        assertEquals(listOf(2L), model.state.value.animes.map { it.id })
    }

    @Test fun logoutThenReconnectDoesNotClearNewSavedConnection() = runTest(main) {
        app.saved = SavedConnection(server.base.toString(), false, "")
        api.setupGate.complete(SetupState(false))
        val delayed = CompletableDeferred<Unit>()
        app.saveGate = delayed
        model.connect(app, server, "", "password")
        main.scheduler.runCurrent()
        assertTrue(app.savesStarted)
        model.logout(app); main.scheduler.runCurrent()
        model.restore(app, SavedConnection(server.base.toString(), false, ""))
        main.scheduler.runCurrent()
        assertNull("logout must suppress restoring the old persisted target", model.connection())
        model.connect(app, ServerConnection.parse("https://new.example/", false), "", "")
        main.scheduler.runCurrent()
        delayed.complete(Unit); main.scheduler.runCurrent()
        assertEquals("https://new.example/", app.saved?.url)
    }

    @Test fun restoreUsesExistingSessionAndNeverCallsLoginWithoutPassword() = runTest(main) {
        api.setupGate.complete(SetupState(false))
        model.restore(app, SavedConnection("https://nas.example/", false, "alice"))
        main.scheduler.runCurrent()
        assertEquals(0, api.loginCalls)
        main.scheduler.runCurrent()
        assertEquals(1, api.probeCalls)
    }

    private suspend fun kotlinx.coroutines.test.TestScope.connect() {
        model.connect(app, server, "", "")
        main.scheduler.runCurrent()
        api.setupGate.complete(SetupState(false)); main.scheduler.runCurrent()
        api.pages.last().complete(AnimePage(emptyList(), 0)); main.scheduler.runCurrent()
    }
}

private fun ClientViewModel.clearForTest() { (this as androidx.lifecycle.ViewModel).let { it.viewModelScope.cancel() } }

private class FakeApi : AnimeApi(OkHttpClient(), ServerConnection.parse("https://nas.example/", false)) {
    var setupGate = CompletableDeferred<SetupState>()
    val pages = mutableListOf<CompletableDeferred<AnimePage>>()
    val details = mutableMapOf<Long, CompletableDeferred<AnimeDetail>>()
    var loginCalls = 0
    var probeCalls = 0
    var nonCancellableDetail = false
    override suspend fun setupStatus() = setupGate.await()
    override suspend fun login(user: String, password: String): LoginResult { loginCalls++; return LoginResult(true) }
    override suspend fun probeSession() { probeCalls++ }
    override suspend fun page(before: Long?): AnimePage = CompletableDeferred<AnimePage>().also { pages += it }.await()
    override suspend fun detail(id: Long): AnimeDetail = if (nonCancellableDetail) withContext(NonCancellable) { details.getOrPut(id) { CompletableDeferred() }.await() } else details.getOrPut(id) { CompletableDeferred() }.await()
}

private class MemoryStorage : CookieStorage {
    private val values = mutableMapOf<String, List<Cookie>>()
    override fun load(origin: String) = Unit
    override fun save(origin: String, cookies: List<Cookie>) { values[origin] = cookies }
    override fun cookies(origin: String) = values[origin].orEmpty()
    override fun clear(origin: String) { values.remove(origin) }
}

private class FakeSessionConnection(override val storage: CookieStorage) : SessionConnection {
    override val generation = AtomicLong()
    override var http: OkHttpClient? = null
    override var streamHttp: OkHttpClient? = null
    var saved: SavedConnection? = null
    var saveGate: CompletableDeferred<Unit>? = null
    var savesStarted = false
    override fun clearSession() { generation.incrementAndGet(); http = null; streamHttp = null }
    override fun connectCovers(client: OkHttpClient) = Unit
    override suspend fun saveConnection(url: String, allowHttp: Boolean, account: String, expectedGeneration: Long) {
        savesStarted = true
        saveGate?.await(); if (generation.get() == expectedGeneration) saved = SavedConnection(url, allowHttp, account)
    }
    override suspend fun clearSavedConnection(expectedGeneration: Long) { if (generation.get() == expectedGeneration) saved = null }
}
