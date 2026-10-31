package com.autoanime.android.feature

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.autoanime.android.SessionConnection
import com.autoanime.android.SavedConnection
import com.autoanime.android.core.Anime
import com.autoanime.android.core.AnimeApi
import com.autoanime.android.core.AnimeDetail
import com.autoanime.android.core.ApiError
import com.autoanime.android.core.ServerConnection
import okhttp3.Cookie
import okhttp3.CookieJar
import okhttp3.HttpUrl
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

data class ClientState(
    val sessionCacheKey: Long = 0,
    val connected: Boolean = false,
    val loading: Boolean = false,
    val error: String? = null,
    val setupRequired: Boolean = false,
    val requiresLogin: Boolean = false,
    val animes: List<Anime> = emptyList(),
    val detail: AnimeDetail? = null,
    val cursor: Long? = null,
    val playbackItemId: Long? = null,
    val playbackPosition: Long = 0,
    val playbackWhenReady: Boolean = true,
)

class ClientViewModel(private val makeApi: (ServerConnection, CookieJar) -> AnimeApi = { server, jar -> AnimeApi(server.newHttpClient(jar), server) }, private val io: kotlinx.coroutines.CoroutineDispatcher = Dispatchers.IO) : ViewModel() {
    private val _state = MutableStateFlow(ClientState())
    val state: StateFlow<ClientState> = _state.asStateFlow()
    private var api: AnimeApi? = null
    private var connection: ServerConnection? = null
    private var request: Job? = null
    private var sessionCookies: List<Cookie> = emptyList()
    private var sessionJar: com.autoanime.android.core.OriginCookieJar? = null
    private var sessionAccount = ""
    @Volatile private var generation = 0L
    private var requestGeneration = 0L
    private var sessionGeneration = 0L
    private var suppressRestore = false
    private var appRef: SessionConnection? = null

    fun restore(app: SessionConnection, saved: SavedConnection) {
        if (connection != null || suppressRestore) return
        runCatching { ServerConnection.parse(saved.url, saved.allowHttp) }
            .onSuccess { connect(app, it, saved.account, "") }
            .onFailure { publishState(ClientState(error = "Saved server URL is invalid. Enter it again.")) }
    }

    fun connect(app: SessionConnection, value: ServerConnection, username: String, password: String) {
        appRef = app
        suppressRestore = false
        sessionJar?.retire()
        val oldClient = app.http
        oldClient?.dispatcher?.cancelAll()
        oldClient?.connectionPool?.evictAll()
        app.clearSession()
        generation = app.generation.get()
        requestGeneration++
        cancelRequest()
        val cookieJar = com.autoanime.android.core.OriginCookieJar(app.storage, value, username)
        sessionJar = cookieJar
        sessionAccount = username
        connection = value
        val service = makeApi(value, cookieJar)
        val client = service.client
        app.http = client
        val stream = value.newHttpClient(cookieJar).newBuilder()
            .readTimeout(0, java.util.concurrent.TimeUnit.MILLISECONDS)
            .callTimeout(0, java.util.concurrent.TimeUnit.MILLISECONDS).build()
        app.connectCovers(client)
        api = service
        sessionGeneration++
        setState(ClientState(loading = true, sessionCacheKey = sessionGeneration))
        val expected = generation
        request = viewModelScope.launch {
            try {
                if (expected != generation) return@launch
                withContext(io) { cookieJar.load() }
                if (expected != generation) return@launch
                val setup = withContext(io) { service.setupStatus() }
                if (setup.required) {
                    publish(expected) { ClientState(setupRequired = true) }
                    return@launch
                }
                if (password.isNotBlank()) withContext(io) { service.login(username, password) }
                if (expected != generation) return@launch
                if (username.isNotBlank() && password.isBlank()) withContext(io) { service.probeSession() }
                if (expected != generation) return@launch
                sessionCookies = cookieJar.activeCookies()
                app.http = client
                app.streamHttp = stream
                withContext(io) { app.saveConnection(value.base.toString(), value.allowHttp, username, expected) }
                publish(expected) { _state.value.copy(connected = true, loading = true, error = null) }
                if (expected == generation) loadPage(expected, service, null, false)
            } catch (e: CancellationException) { throw e }
            catch (e: Exception) {
                if (expected == generation && e is ApiError && e.status == 401) expireSessionError(app)
                else if (expected == generation) {
                    app.clearSession()
                    api = null; connection = null; sessionJar?.retire(); sessionJar = null
                    publish(expected) { ClientState(error = safeMessage(e), sessionCacheKey = sessionGeneration) }
                }
            }
        }
    }

    fun loadPage(append: Boolean = false) {
        val service = api ?: return
        loadPage(generation, service, if (append) _state.value.cursor else null, append)
    }

    private fun loadPage(expected: Long, service: AnimeApi, before: Long?, append: Boolean) {
        val requestId = ++requestGeneration
        cancelRequest()
        setState(_state.value.copy(loading = true, error = null))
        request = viewModelScope.launch {
            try {
                val page = withContext(io) { service.page(before) }
                publish(expected) {
                    if (requestId != requestGeneration) return@publish _state.value
                    val old = if (append) _state.value.animes else emptyList()
                    val ids = old.mapTo(mutableSetOf()) { it.id }
                    _state.value.copy(loading = false, animes = old + page.items.filter { ids.add(it.id) }, cursor = page.next_before.takeIf { it > 0 }, error = null)
                }
            } catch (e: CancellationException) { throw e }
            catch (e: Exception) {
                if (requestId == requestGeneration && e is ApiError && e.status == 401) expireSessionError()
                else publish(expected) { if (requestId == requestGeneration) _state.value.copy(loading = false, error = safeMessage(e)) else _state.value }
            }
        }
    }

    fun loadDetail(id: Long) {
        val service = api ?: return
        cancelRequest()
        val expected = generation
        val requestId = ++requestGeneration
        setState(_state.value.copy(loading = true, error = null))
        request = viewModelScope.launch {
            try {
                val value = withContext(io) { service.detail(id) }
                publish(expected) { if (requestId == requestGeneration) _state.value.copy(loading = false, detail = value, error = null) else _state.value }
            } catch (e: CancellationException) { throw e }
            catch (e: Exception) {
                if (requestId == requestGeneration && e is ApiError && e.status == 401) expireSessionError()
                else publish(expected) { if (requestId == requestGeneration) _state.value.copy(loading = false, error = safeMessage(e)) else _state.value }
            }
        }
    }

    fun restoreDetail(id: Long) {
        if (_state.value.detail?.id != id && connection != null) loadDetail(id)
    }

    fun clearDetail(id: Long) {
        if (_state.value.detail?.id == id) setState(_state.value.copy(detail = null, playbackItemId = null, playbackPosition = 0))
    }

    fun expireSession(app: SessionConnection) {
        if (_state.value.requiresLogin) { app.clearSession(); return }
        sessionJar?.retireAndClear()
        requestGeneration++
        cancelRequest()
        api = null
        connection = null
        sessionCookies = emptyList()
        app.clearSession()
        sessionJar = null
        suppressRestore = true
        setState(ClientState(error = "Session expired. Log in again.", requiresLogin = true, sessionCacheKey = ++sessionGeneration))
    }

    private fun expireSessionError(app: SessionConnection? = appRef) {
        sessionJar?.retireAndClear()
        requestGeneration++
        cancelRequest()
        api = null
        connection = null
        sessionCookies = emptyList()
        app?.clearSession()
        suppressRestore = true
        if (_state.value.requiresLogin) return
        setState(ClientState(error = "Session expired. Log in again.", requiresLogin = true, sessionCacheKey = ++sessionGeneration))
    }

    fun disconnectForExpiredSession(app: SessionConnection) = expireSession(app)

    fun setPlayback(itemId: Long?) {
        setState(_state.value.copy(playbackItemId = itemId, playbackPosition = 0, playbackWhenReady = true))
    }

    fun savePlayback(position: Long, whenReady: Boolean) {
        setState(_state.value.copy(playbackPosition = position.coerceAtLeast(0), playbackWhenReady = whenReady))
    }

    fun logout(app: SessionConnection) {
        val expected = ++generation
        suppressRestore = true
        requestGeneration++
        val oldApi = api
        val oldConnection = connection
        val oldCookies = sessionCookies
        val oldClient = app.http
        cancelRequest()
        app.http?.dispatcher?.cancelAll()
        app.http?.connectionPool?.evictAll()
        app.http = null
        app.streamHttp = null
        sessionJar?.retireAndClear()
        sessionJar = null
        sessionAccount = ""
        app.clearSession()
        val persistenceGeneration = app.generation.get()
        api = null
        connection = null
        sessionCookies = emptyList()
        setState(ClientState(sessionCacheKey = ++sessionGeneration))
        viewModelScope.launch { withContext(io) { app.clearSavedConnection(persistenceGeneration) } }
        request = viewModelScope.launch {
            if (generation != expected) return@launch
            if (oldApi != null && oldConnection != null && generation == expected) {
                val isolated = oldConnection.newHttpClient(object : CookieJar {
                    override fun loadForRequest(url: HttpUrl) = oldCookies.filter { it.matches(url) }
                    override fun saveFromResponse(url: HttpUrl, cookies: List<Cookie>) = Unit
                })
                try { withContext(io) { AnimeApi(isolated, oldConnection).logout() } } catch (_: Exception) { }
                isolated.dispatcher.cancelAll()
                oldClient?.connectionPool?.evictAll()
            }
        }
    }

    fun connection() = connection
    fun api() = api

    @Synchronized private fun cancelRequest() { request?.cancel(); request = null }
    private fun setState(value: ClientState) {
        if (kotlinx.coroutines.Dispatchers.Main.immediate.isDispatchNeeded(kotlin.coroutines.EmptyCoroutineContext)) viewModelScope.launch(Dispatchers.Main.immediate) { _state.value = value }
        else _state.value = value
    }
    private fun publishState(value: ClientState) = setState(value)
    private suspend fun publish(expected: Long, update: () -> ClientState) = withContext(Dispatchers.Main) {
        if (expected == generation) _state.value = update()
    }
    private fun safeMessage(e: Exception) = when (e) {
        is ApiError -> if (e.status == 401) "Session expired. Log in again." else "Request failed (${e.status})"
        is kotlinx.serialization.SerializationException -> "Server update required: this server does not support the mobile library API."
        else -> e.message?.takeIf { it == "Response is too large" } ?: "Connection failed. Check server and network."
    }
}
