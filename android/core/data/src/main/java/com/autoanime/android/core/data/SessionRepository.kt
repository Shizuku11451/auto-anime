package com.autoanime.android.core.data

import coil3.ImageLoader
import com.autoanime.android.core.model.AppError
import com.autoanime.android.core.network.AnimeApi
import com.autoanime.android.core.network.CookieStorage
import com.autoanime.android.core.network.OriginCookieJar
import com.autoanime.android.core.network.ServerConnection
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import okhttp3.Cookie
import okhttp3.CookieJar
import okhttp3.HttpUrl
import okhttp3.OkHttpClient
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicLong
import javax.inject.Inject
import javax.inject.Singleton

sealed interface SessionState {
    /** Startup restore of a saved connection is still running. */
    data object Restoring : SessionState

    /** No usable session. [form] pre-fills the connect screen; it never holds a password. */
    data class SignedOut(
        val form: SavedConnection? = null,
        val connecting: Boolean = false,
        val error: AppError? = null,
    ) : SessionState

    data class Connected(val session: Session) : SessionState
}

fun interface ApiFactory {
    fun create(server: ServerConnection, cookies: CookieJar): AnimeApi
}

fun interface CoverImageLoaderFactory {
    fun create(client: OkHttpClient): ImageLoader
}

/**
 * One authenticated connection. All of its clients, cookies and cached covers are discarded
 * together when it ends, so nothing leaks into the next server or account.
 */
class Session internal constructor(
    val id: Long,
    val connection: ServerConnection,
    val account: String,
    val api: AnimeApi,
    /** No read or call timeout: media requests stay open while playing. */
    val streamClient: OkHttpClient,
    internal val cookies: OriginCookieJar,
    imageLoaderFactory: CoverImageLoaderFactory,
) {
    private val coverClient = api.client.newBuilder().callTimeout(15, TimeUnit.SECONDS).build()
    private val imageLoaderLazy = lazy { imageLoaderFactory.create(coverClient) }
    val imageLoader: ImageLoader by imageLoaderLazy

    fun resolve(path: String): HttpUrl = connection.resolve(path)

    internal fun close() {
        listOf(api.client, streamClient, coverClient).forEach {
            it.dispatcher.cancelAll()
            it.connectionPool.evictAll()
        }
        if (imageLoaderLazy.isInitialized()) {
            imageLoader.memoryCache?.clear()
            imageLoader.shutdown()
        }
    }
}

/**
 * Owns the connection lifecycle. Every connect, logout and expiry advances [generation];
 * work started for an older generation can finish but never publishes state or persists.
 */
@Singleton
class SessionRepository @Inject constructor(
    private val cookieStorage: dagger.Lazy<CookieStorage>,
    private val connectionStore: ConnectionStore,
    private val apiFactory: ApiFactory,
    private val imageLoaderFactory: CoverImageLoaderFactory,
    @ApplicationScope private val scope: CoroutineScope,
    @IoDispatcher private val io: CoroutineDispatcher,
) {
    private val _state = MutableStateFlow<SessionState>(SessionState.Restoring)
    val state: StateFlow<SessionState> = _state.asStateFlow()

    private val generation = AtomicLong()
    private val persistence = Mutex()
    private var restoreStarted = false
    private var connectJob: Job? = null
    private var current: Session? = null

    /** Restores the saved connection once per process, reusing its stored session cookie. */
    @Synchronized
    fun restore() {
        if (restoreStarted) return
        restoreStarted = true
        val expected = generation.get()
        scope.launch {
            val saved = withContext(io) { connectionStore.load() }
            synchronized(this@SessionRepository) {
                if (generation.get() != expected) return@launch
                if (saved == null) {
                    _state.value = SessionState.SignedOut()
                    return@launch
                }
                val server = runCatching { ServerConnection.parse(saved.url, saved.allowHttp) }.getOrNull()
                if (server == null) {
                    _state.value = SessionState.SignedOut(saved, error = AppError.InvalidSavedUrl)
                    return@launch
                }
                connect(server, saved.account, password = "")
            }
        }
    }

    @Synchronized
    fun connect(server: ServerConnection, account: String, password: String) {
        val expected = generation.incrementAndGet()
        restoreStarted = true
        connectJob?.cancel()
        endCurrent(clearCookies = false)
        val form = SavedConnection(server.base.toString(), server.allowHttp, account)
        _state.value = SessionState.SignedOut(form, connecting = true)
        connectJob = scope.launch {
            var jar: OriginCookieJar? = null
            var api: AnimeApi? = null
            try {
                // Loading may touch Android Keystore, so it stays off the main thread.
                jar = withContext(io) { OriginCookieJar(cookieStorage.get(), server, account).also { it.load() } }
                val service = apiFactory.create(server, jar).also { api = it }
                val setup = withContext(io) { service.setupStatus() }
                if (setup.required) {
                    fail(expected, jar, service, form, AppError.SetupRequired, clearCookies = false)
                    return@launch
                }
                if (password.isNotBlank()) withContext(io) { service.login(account, password) }
                else if (account.isNotBlank()) withContext(io) { service.probeSession() }
                val stream = server.newHttpClient(jar).newBuilder()
                    .readTimeout(0, TimeUnit.MILLISECONDS)
                    .callTimeout(0, TimeUnit.MILLISECONDS)
                    .build()
                val session = Session(expected, server, account, service, stream, jar, imageLoaderFactory)
                persist(expected) { connectionStore.save(form) }
                synchronized(this@SessionRepository) {
                    if (generation.get() != expected) {
                        jar.retire()
                        session.close()
                        return@launch
                    }
                    current = session
                    _state.value = SessionState.Connected(session)
                }
            } catch (e: CancellationException) {
                jar?.retire()
                api?.client?.dispatcher?.cancelAll()
                throw e
            } catch (e: Exception) {
                val error = e.toAppError()
                fail(expected, jar, api, form, error, clearCookies = error == AppError.SessionExpired)
            }
        }
    }

    /** Ends [session] after a 401. Repeated reports for the same session publish one transition. */
    @Synchronized
    fun onUnauthorized(session: Session) {
        if (current !== session) return
        generation.incrementAndGet()
        endCurrent(clearCookies = true)
        _state.value = SessionState.SignedOut(session.form(), error = AppError.SessionExpired)
    }

    @Synchronized
    fun logout() {
        val expected = generation.incrementAndGet()
        connectJob?.cancel()
        val session = current
        val snapshot = session?.cookies?.activeCookies().orEmpty()
        endCurrent(clearCookies = true)
        _state.value = SessionState.SignedOut(session?.form())
        scope.launch { persist(expected) { connectionStore.clear() } }
        if (session != null) scope.launch {
            // Best effort: the server session is revoked with the cookies captured before clearing.
            val isolated = object : CookieJar {
                override fun loadForRequest(url: HttpUrl): List<Cookie> = snapshot.filter { it.matches(url) }
                override fun saveFromResponse(url: HttpUrl, cookies: List<Cookie>) = Unit
            }
            val api = apiFactory.create(session.connection, isolated)
            try { withContext(io) { api.logout() } } catch (e: CancellationException) { throw e } catch (_: Exception) { }
            api.client.dispatcher.cancelAll()
            api.client.connectionPool.evictAll()
        }
    }

    private fun endCurrent(clearCookies: Boolean) {
        val session = current ?: return
        current = null
        if (clearCookies) session.cookies.retireAndClear() else session.cookies.retire()
        session.close()
    }

    private fun fail(expected: Long, jar: OriginCookieJar?, api: AnimeApi?, form: SavedConnection, error: AppError, clearCookies: Boolean) {
        if (clearCookies) jar?.retireAndClear() else jar?.retire()
        api?.client?.dispatcher?.cancelAll()
        api?.client?.connectionPool?.evictAll()
        synchronized(this) {
            if (generation.get() == expected) _state.value = SessionState.SignedOut(form, error = error)
        }
    }

    /** Serializes saved-connection writes; a write from a superseded generation is skipped. */
    private suspend fun persist(expected: Long, block: suspend () -> Unit) = persistence.withLock {
        if (generation.get() == expected) withContext(io) { block() }
    }

    private fun Session.form() = SavedConnection(connection.base.toString(), connection.allowHttp, account)
}
