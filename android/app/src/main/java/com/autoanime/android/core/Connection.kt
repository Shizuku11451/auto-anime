package com.autoanime.android.core

import okhttp3.Cookie
import okhttp3.CookieJar
import okhttp3.HttpUrl
import okhttp3.OkHttpClient
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import java.util.concurrent.TimeUnit

data class ServerConnection(val base: HttpUrl, val allowHttp: Boolean) {
    companion object {
        fun parse(raw: String, allowHttp: Boolean): ServerConnection {
            val url = raw.trim().toHttpUrlOrNullCompat()
                ?: throw IllegalArgumentException("Enter a valid server URL")
            require(url.encodedUsername.isEmpty() && url.encodedPassword.isEmpty()) { "Credentials are not allowed in the URL" }
            require(url.query == null && url.fragment == null) { "Query and fragment are not allowed" }
            require(url.encodedPath.endsWith("/")) { "Server path must end with /" }
            require(url.scheme == "https" || (url.scheme == "http" && allowHttp)) { "HTTP requires explicit confirmation" }
            return ServerConnection(url, allowHttp)
        }
    }

    fun resolve(path: String): HttpUrl = base.resolve(path.removePrefix("/"))
        ?: throw IllegalArgumentException("Invalid API path")

    fun accepts(url: HttpUrl): Boolean = origin(base) == origin(url) &&
        (base.scheme != "https" || url.scheme == "https")

    fun requireAccepted(url: HttpUrl) {
        require(accepts(url)) { "Request escaped the selected server origin" }
    }

    fun newHttpClient(jar: CookieJar): OkHttpClient = OkHttpClient.Builder()
        .dispatcher(okhttp3.Dispatcher().apply { maxRequests = 8; maxRequestsPerHost = 4 })
        .cookieJar(jar)
        .connectTimeout(10, TimeUnit.SECONDS)
        .readTimeout(20, TimeUnit.SECONDS)
        .callTimeout(30, TimeUnit.SECONDS)
        .followRedirects(false)
        .followSslRedirects(false)
        .addInterceptor { chain ->
            val request = chain.request()
            requireAccepted(request.url)
            chain.proceed(request)
        }
        .build()

    private fun origin(url: HttpUrl) = "${url.scheme}://${url.host}:${url.port}"
}

private fun String.toHttpUrlOrNullCompat(): HttpUrl? = toHttpUrlOrNull()

/** Session cookies are keyed by the complete origin, never shared between servers. */
class OriginCookieJar(private val storage: CookieStorage, private val connection: ServerConnection, account: String = "") : CookieJar {
    private val origin = "${connection.base.origin().length}:${connection.base.origin()}${account.length}:$account"
    private val serverOrigin = connection.base.origin()
    @Volatile private var retired = false
    // Loading may touch Android Keystore; callers do this from their IO coroutine.
    @Synchronized fun load() { if (!retired) storage.load(origin) }
    @Synchronized fun retire() { retired = true }
    @Synchronized fun retireAndClear() {
        retired = true
        storage.clear(origin)
    }
    @Synchronized override fun saveFromResponse(url: HttpUrl, cookies: List<Cookie>) {
        if (retired || url.origin() != serverOrigin) return
        val incoming = cookies.filter {
            it.name == "auto_anime_session" &&
                (it.domain == connection.base.host || (!it.hostOnly && connection.base.host.endsWith(".${it.domain}"))) &&
                (!it.secure || connection.base.isHttps)
        }
        val current = storage.cookies(origin).toMutableList()
        incoming.forEach { cookie ->
            current.removeAll { it.name == cookie.name && it.domain == cookie.domain && it.path == cookie.path }
            if (cookie.expiresAt > System.currentTimeMillis()) current.add(cookie)
        }
        if (incoming.isNotEmpty()) storage.save(origin, current)
    }
    @Synchronized override fun loadForRequest(url: HttpUrl): List<Cookie> =
        if (!retired && url.origin() == serverOrigin) storage.cookies(origin).filter { it.matches(url) } else emptyList()
    @Synchronized fun activeCookies(): List<Cookie> = if (retired) emptyList() else storage.cookies(origin)
        .filter { it.expiresAt > System.currentTimeMillis() && it.matches(connection.base.newBuilder().encodedPath(it.path).build()) }.toList()
    fun clearSession() { storage.clear(origin) }

    private fun HttpUrl.origin() = "$scheme://$host:$port"
}

interface CookieStorage {
    fun load(origin: String)
    fun save(origin: String, cookies: List<Cookie>)
    fun cookies(origin: String): List<Cookie>
    fun clear(origin: String)
}

private fun HttpUrl.origin() = "$scheme://$host:$port"
