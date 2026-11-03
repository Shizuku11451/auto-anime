package com.autoanime.android.feature.discover

import com.autoanime.android.core.data.ConnectionStore
import com.autoanime.android.core.data.SavedConnection
import com.autoanime.android.core.data.SessionRepository
import com.autoanime.android.core.model.SetupState
import com.autoanime.android.core.network.AnimeApi
import com.autoanime.android.core.network.CookieStorage
import com.autoanime.android.core.network.ServerConnection
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import okhttp3.Cookie
import okhttp3.OkHttpClient

/** A connected [SessionRepository] whose API is [api]; setup and session probes always succeed. */
internal fun TestScope.connectedSessions(dispatcher: CoroutineDispatcher, api: AnimeApi): SessionRepository {
    val sessions = SessionRepository(
        cookieStorage = { MemoryCookieStorage() },
        connectionStore = NoopConnectionStore,
        apiFactory = { _, _ -> api },
        imageLoaderFactory = { error("covers are not loaded in unit tests") },
        scope = backgroundScope,
        io = dispatcher,
    )
    sessions.connect(ServerConnection.parse("https://nas.example/", false), "", "")
    runCurrent()
    return sessions
}

/** Base for fakes: answers the calls a connect makes. */
internal open class BaseFakeApi : AnimeApi(OkHttpClient(), ServerConnection.parse("https://nas.example/", false)) {
    override suspend fun setupStatus() = SetupState(false)
    override suspend fun probeSession() = Unit
}

private object NoopConnectionStore : ConnectionStore {
    override suspend fun load(): SavedConnection? = null
    override suspend fun save(connection: SavedConnection) = Unit
    override suspend fun clear() = Unit
}

private class MemoryCookieStorage : CookieStorage {
    private val values = mutableMapOf<String, List<Cookie>>()
    override fun load(origin: String) = Unit
    override fun save(origin: String, cookies: List<Cookie>) { values[origin] = cookies }
    override fun cookies(origin: String) = values[origin].orEmpty()
    override fun clear(origin: String) { values.remove(origin) }
}
