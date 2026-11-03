package com.autoanime.android.core.network

import okhttp3.Cookie
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.Request
import org.junit.Assert.*
import org.junit.Test

class ConnectionTest {
    @Test fun preservesProxyPrefixAndRequiresExplicitHttp() {
        val https = ServerConnection.parse("https://nas.example/aa/proxy/", false)
        assertEquals("https://nas.example/aa/proxy/api/items", https.resolve("api/items").toString())
        assertFalse(https.accepts("http://nas.example/aa/proxy/api/items".toHttpUrl()))
        assertTrue(runCatching { ServerConnection.parse("http://nas.example/aa/", false) }.isFailure)
        assertEquals("http://nas.example/aa/", ServerConnection.parse("http://nas.example/aa/", true).base.toString())
    }

    @Test fun rejectsEscapedOriginsAsIoFailure() {
        val connection = ServerConnection.parse("https://nas.example/nas/", false)
        assertTrue(runCatching { connection.requireAccepted("https://evil.example/path".toHttpUrl()) }.exceptionOrNull() is java.io.IOException)
    }

    @Test fun asyncClientReportsEscapedOriginWithoutConnecting() {
        val connection = ServerConnection.parse("https://nas.example/nas/", false)
        val client = connection.newHttpClient(object : okhttp3.CookieJar {
            override fun saveFromResponse(url: okhttp3.HttpUrl, cookies: List<Cookie>) = Unit
            override fun loadForRequest(url: okhttp3.HttpUrl) = emptyList<Cookie>()
        })
        val failure = java.util.concurrent.CountDownLatch(1)
        var result: java.io.IOException? = null
        client.newCall(Request.Builder().url("http://127.0.0.1:1/outside").build()).enqueue(object : okhttp3.Callback {
            override fun onFailure(call: okhttp3.Call, e: java.io.IOException) { result = e; failure.countDown() }
            override fun onResponse(call: okhttp3.Call, response: okhttp3.Response) { response.close(); failure.countDown() }
        })
        assertTrue("callback should finish", failure.await(5, java.util.concurrent.TimeUnit.SECONDS))
        assertTrue(result is java.io.IOException)
        assertEquals("Request escaped the selected server origin", result?.message)
    }

    @Test fun cookieJarDoesNotLeakBetweenPortsOrHosts() {
        val storage = MemoryStorage()
        val a = ServerConnection.parse("https://one.example:8443/nas/", false)
        val b = ServerConnection.parse("https://one.example:9443/nas/", false)
        val jarA = OriginCookieJar(storage, a, "alice")
        jarA.saveFromResponse(a.base, listOf(Cookie.Builder().name("auto_anime_session").value("secret").hostOnlyDomain("one.example").path("/").build()))
        assertEquals(1, jarA.loadForRequest(a.base).size)
        assertTrue(OriginCookieJar(storage, b, "alice").loadForRequest(b.base).isEmpty())
        assertTrue(OriginCookieJar(storage, a, "bob").loadForRequest(a.base).isEmpty())
        assertEquals("secret", OriginCookieJar(storage, a, "alice").loadForRequest(a.base).single().value)
    }

    @Test fun cookiesAreScopedBySchemePathExpiryPortAndAccount() {
        val storage = MemoryStorage()
        val connection = ServerConnection.parse("https://one.example:8443/nas/", false)
        val jar = OriginCookieJar(storage, connection, "alice")
        assertTrue(jar.loadForRequest(connection.resolve("api/items")).isEmpty())
        val apiCookie = Cookie.Builder().name("auto_anime_session").value("test-only-api")
            .hostOnlyDomain("one.example").path("/nas/api/").build()
        jar.saveFromResponse(connection.base, listOf(apiCookie))
        assertEquals(listOf("test-only-api"), jar.loadForRequest(connection.resolve("api/items")).map { it.value })
        assertTrue(jar.loadForRequest(connection.resolve("other")).isEmpty())
        assertTrue(jar.loadForRequest("http://one.example:8443/nas/api/items".toHttpUrl()).isEmpty())
        assertTrue(OriginCookieJar(storage, connection, "bob").activeCookies().isEmpty())
        assertTrue(OriginCookieJar(storage, ServerConnection.parse("https://one.example:9443/nas/", false), "alice").activeCookies().isEmpty())

        val expired = Cookie.Builder().name("auto_anime_session").value("expired")
            .hostOnlyDomain("one.example").path("/nas/api/").expiresAt(System.currentTimeMillis() - 1).build()
        jar.saveFromResponse(connection.base, listOf(expired))
        assertTrue(jar.loadForRequest(connection.base).isEmpty())
        assertTrue(jar.activeCookies().isEmpty())
    }

    @Test fun apiPathCookieAppearsInSessionSnapshotForLogout() {
        val connection = ServerConnection.parse("https://one.example:8443/nas/", false)
        val jar = OriginCookieJar(MemoryStorage(), connection, "alice")
        jar.saveFromResponse(connection.resolve("api/login"), listOf(Cookie.Builder()
            .name("auto_anime_session").value("test-only")
            .hostOnlyDomain("one.example").path("/nas/api/").build()))
        assertEquals("test-only", jar.activeCookies().single().value)
        assertEquals("test-only", jar.loadForRequest(connection.resolve("api/logout")).single().value)
    }

    @Test fun cookieLoadIsExplicitAndOriginScoped() {
        val storage = MemoryStorage()
        val connection = ServerConnection.parse("https://one.example/nas/", false)
        val first = OriginCookieJar(storage, connection, "alice")
        first.saveFromResponse(connection.base, listOf(Cookie.Builder().name("auto_anime_session").value("test-only")
            .hostOnlyDomain("one.example").path("/").build()))
        val persistedStorage = MemoryStorage()
        val persisted = OriginCookieJar(persistedStorage, connection, "alice")
        persisted.saveFromResponse(connection.base, listOf(Cookie.Builder().name("auto_anime_session").value("persisted-test-only")
            .hostOnlyDomain("one.example").path("/").build()))
        val reloaded = OriginCookieJar(storage, connection, "alice")
        assertEquals("test-only", reloaded.activeCookies().single().value)
        val restored = OriginCookieJar(persistedStorage, connection, "alice")
        restored.load()
        assertEquals("persisted-test-only", restored.activeCookies().single().value)
        reloaded.load()
        assertEquals("test-only", reloaded.activeCookies().single().value)
    }

    @Test fun reverseProxyCoverPathRemainsUnderPrefix() {
        val connection = ServerConnection.parse("https://nas.example/nas/root/", false)
        assertEquals("https://nas.example/nas/root/api/covers/poster.jpg", connection.resolve("/api/covers/poster.jpg").toString())
    }

    @Test fun accountIdentityEncodingCannotCollideOnSeparators() {
        val storage = MemoryStorage()
        val connection = ServerConnection.parse("https://one.example/", false)
        val alice = OriginCookieJar(storage, connection, "a|b:c")
        alice.saveFromResponse(connection.base, listOf(Cookie.Builder().name("auto_anime_session").value("test-only")
            .hostOnlyDomain("one.example").path("/").build()))
        assertTrue(OriginCookieJar(storage, connection, "a|b").activeCookies().isEmpty())
        assertTrue(OriginCookieJar(storage, connection, "a").activeCookies().isEmpty())
        assertEquals("test-only", OriginCookieJar(storage, connection, "a|b:c").activeCookies().single().value)
    }

    @Test fun cookieRemovalAndRetirementAreAtomicAgainstConcurrentResponseSave() {
        val storage = MemoryStorage()
        val connection = ServerConnection.parse("https://one.example/", false)
        val jar = OriginCookieJar(storage, connection, "alice")
        val cookie = Cookie.Builder().name("auto_anime_session").value("test-only")
            .hostOnlyDomain("one.example").path("/").build()
        jar.saveFromResponse(connection.base, listOf(cookie))
        val start = java.util.concurrent.CountDownLatch(1)
        val done = java.util.concurrent.CountDownLatch(2)
        Thread { start.await(); jar.saveFromResponse(connection.base, listOf(cookie)); done.countDown() }.start()
        Thread { start.await(); jar.retireAndClear(); done.countDown() }.start()
        start.countDown()
        assertTrue(done.await(2, java.util.concurrent.TimeUnit.SECONDS))
        assertTrue(jar.activeCookies().isEmpty())
        assertTrue(jar.loadForRequest(connection.base).isEmpty())
        assertTrue(storage.cookiesForTest().isEmpty())
    }

    private class MemoryStorage : CookieStorage {
        private val values = mutableMapOf<String, List<Cookie>>()
        override fun load(origin: String) { }
        override fun save(origin: String, cookies: List<Cookie>) { values[origin] = cookies }
        override fun cookies(origin: String) = values[origin].orEmpty()
        override fun clear(origin: String) { values.remove(origin) }
        fun cookiesForTest() = values.values.flatten()
    }
}
