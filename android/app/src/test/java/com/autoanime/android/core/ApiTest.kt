package com.autoanime.android.core

import kotlinx.serialization.json.Json
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.Assert.*
import org.junit.Test
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.async
import okhttp3.Request

class ApiTest {
    @Test fun pageAcceptsUnknownFieldsAndEmptyCursor() {
        val page = Json { ignoreUnknownKeys = true }.decodeFromString<AnimePage>("""{"items":[{"id":7,"title":"番剧","cover":"/api/covers/a.jpg","extra":"ignored"}],"next_before":0}""")
        assertEquals("番剧", page.items.single().title)
        assertEquals("/api/covers/a.jpg", page.items.single().cover)
        assertEquals(0L, page.next_before)
    }

    @Test fun requestsKeepReverseProxyPrefixAndDoNotFollowRedirects() = runBlocking {
        val server = MockWebServer()
        server.start()
        try {
            server.enqueue(MockResponse().setResponseCode(302).addHeader("Location", "https://other.example/api/setup/status"))
            val connection = ServerConnection.parse(server.url("/anime/").toString(), allowHttp = true)
            val api = AnimeApi(connection.newHttpClient(OriginCookieJar(MemoryCookieStorage(), connection)), connection)
            try {
                api.setupStatus()
                fail("redirect response must not be followed")
            } catch (e: ApiError) { assertEquals(302, e.status) }
            assertEquals("/anime/api/setup/status", server.takeRequest().path)
            assertEquals(1, server.requestCount)
        } finally { server.shutdown() }
    }

    @Test fun successfulApiRequestUsesConfiguredReverseProxyPrefix() = runBlocking {
        val server = MockWebServer().apply { start() }
        try {
            server.enqueue(MockResponse().setBody("""{"required":false,"unknown":"ignored"}"""))
            val connection = ServerConnection.parse(server.url("/nas/root/").toString(), true)
            val api = AnimeApi(connection.newHttpClient(OriginCookieJar(MemoryCookieStorage(), connection)), connection)
            assertFalse(api.setupStatus().required)
            assertEquals("/nas/root/api/setup/status", server.takeRequest().path)
        } finally { server.shutdown() }
    }

    @Test fun rejectsOversizedApiResponses() = runBlocking {
        val server = MockWebServer()
        server.start()
        try {
            server.enqueue(MockResponse().setBody("x".repeat(1_048_577)))
            val connection = ServerConnection.parse(server.url("/").toString(), allowHttp = true)
            val api = AnimeApi(OkHttpClient.Builder().followRedirects(false).build(), connection)
            try {
                api.setupStatus()
                fail("oversized response must be rejected")
            } catch (e: IllegalArgumentException) { assertEquals("Response is too large", e.message) }
        } finally { server.shutdown() }
    }

    @Test fun loginStoresOnlySessionCookieAnd401IsActionable() = runBlocking {
        val server = MockWebServer()
        server.start()
        try {
            server.enqueue(MockResponse().addHeader("Set-Cookie", "auto_anime_session=secret; Path=/; HttpOnly").setBody("""{"ok":true}"""))
            server.enqueue(MockResponse().setResponseCode(401).setBody("expired"))
            val connection = ServerConnection.parse(server.url("/proxy/").toString(), true)
            val storage = MemoryCookieStorage()
            val jar = OriginCookieJar(storage, connection, "alice|one")
            val api = AnimeApi(connection.newHttpClient(jar), connection)
            assertTrue(api.login("alice", "pass").ok)
            assertEquals("secret", jar.activeCookies().single().value)
            try { api.probeSession(); fail("expired session must be rejected") }
            catch (e: ApiError) { assertEquals(401, e.status) }
            assertEquals("/proxy/api/login", server.takeRequest().path)
            assertEquals("/proxy/api/animes?page=1&limit=1", server.takeRequest().path)
        } finally { server.shutdown() }
    }

    @Test fun logoutMaxAgeZeroCanBeClearedFromSessionJar() = runBlocking {
        val server = MockWebServer().apply { start() }
        try {
            server.enqueue(MockResponse().addHeader("Set-Cookie", "auto_anime_session=test-only; Path=/; HttpOnly").setBody("""{"ok":true}"""))
            server.enqueue(MockResponse().addHeader("Set-Cookie", "auto_anime_session=; Path=/; Max-Age=0; HttpOnly"))
            val connection = ServerConnection.parse(server.url("/proxy/").toString(), true)
            val jar = OriginCookieJar(MemoryCookieStorage(), connection, "test-account")
            val api = AnimeApi(connection.newHttpClient(jar), connection)
            api.login("test-user", "test-password")
            assertEquals("test-only", jar.activeCookies().single().value)
            api.logout()
            jar.retireAndClear()
            assertTrue(jar.activeCookies().isEmpty())
        } finally { server.shutdown() }
    }

    @Test fun responseWithoutSetCookieDoesNotClearCurrentSession() = runBlocking {
        val server = MockWebServer().apply { start() }
        try {
            server.enqueue(MockResponse().addHeader("Set-Cookie", "auto_anime_session=test-only; Path=/; HttpOnly").setBody("""{"ok":true}"""))
            server.enqueue(MockResponse().setBody("""{"required":false}"""))
            val connection = ServerConnection.parse(server.url("/proxy/").toString(), true)
            val jar = OriginCookieJar(MemoryCookieStorage(), connection, "test-account")
            val api = AnimeApi(connection.newHttpClient(jar), connection)
            api.login("test-user", "test-password")
            assertFalse(api.setupStatus().required)
            assertEquals("test-only", jar.activeCookies().single().value)
            server.takeRequest()
            assertEquals("auto_anime_session=test-only", server.takeRequest().getHeader("Cookie"))
        } finally { server.shutdown() }
    }

    @Test fun cookieDomainAndPathCannotMakeSessionCookieReachAnotherHostOrPath() = runBlocking {
        val server = MockWebServer().apply { start() }
        try {
            val connection = ServerConnection.parse(server.url("/proxy/").toString(), true)
            val jar = OriginCookieJar(MemoryCookieStorage(), connection, "test-account")
            val host = connection.base.host
            val root = okhttp3.Cookie.Builder().name("auto_anime_session").value("test-only-domain")
                .hostOnlyDomain(host).path("/").build()
            val outsidePath = okhttp3.Cookie.Builder().name("auto_anime_session").value("test-only-path")
                .hostOnlyDomain(host).path("/outside/").build()
            val otherHost = connection.base.newBuilder().host("elsewhere.invalid").build()
            val withRoot = OriginCookieJar(MemoryCookieStorage(), connection, "root-cookie")
            withRoot.saveFromResponse(connection.base, listOf(root))
            assertTrue(withRoot.loadForRequest(otherHost).isEmpty())
            assertEquals("test-only-domain", withRoot.loadForRequest(connection.resolve("api/setup/status")).single().value)
            val withOutsidePath = OriginCookieJar(MemoryCookieStorage(), connection, "path-cookie")
            withOutsidePath.saveFromResponse(connection.base, listOf(outsidePath))
            assertTrue(withOutsidePath.loadForRequest(connection.resolve("api/setup/status")).isEmpty())
            assertTrue(withOutsidePath.loadForRequest(connection.base).isEmpty())
        } finally { server.shutdown() }
    }

    @Test fun rejectsRequestEscapingSelectedOrigin() = runBlocking {
        val server = MockWebServer()
        server.start()
        try {
            val connection = ServerConnection.parse(server.url("/proxy/").toString(), true)
            val client = connection.newHttpClient(OriginCookieJar(MemoryCookieStorage(), connection))
            try { client.newCall(Request.Builder().url(server.url("/escaped")).build().newBuilder().url("http://other.example/api/animes/1").build()).execute().use { fail("cross-origin request must be rejected") } }
            catch (e: IllegalArgumentException) { assertEquals("Request escaped the selected server origin", e.message) }
            assertEquals(0, server.requestCount)
        } finally { server.shutdown() }
    }

    @Test fun cancellingSuspendingRequestCancelsUnderlyingCall() = runBlocking {
        val server = MockWebServer()
        server.start()
        try {
            server.enqueue(MockResponse().setHeadersDelay(5, java.util.concurrent.TimeUnit.SECONDS).setBody("{}"))
            val connection = ServerConnection.parse(server.url("/").toString(), allowHttp = true)
            val client = connection.newHttpClient(OriginCookieJar(MemoryCookieStorage(), connection))
            val api = AnimeApi(client, connection)
            val job = launch(Dispatchers.IO) { api.setupStatus() }
            server.takeRequest()
            job.cancel()
            job.join()
            withTimeout(2_000) { while (client.dispatcher.runningCallsCount() != 0) kotlinx.coroutines.delay(10) }
            assertEquals(0, client.dispatcher.runningCallsCount())
        } finally { server.shutdown() }
    }

    @Test fun cancellationAfterHeadersCancelsBodyReadAndReleasesCall() = runBlocking {
        val server = MockWebServer().apply { start() }
        val headersReceived = java.util.concurrent.CountDownLatch(1)
        try {
            server.enqueue(MockResponse().setBodyDelay(10, java.util.concurrent.TimeUnit.SECONDS)
                .setBody("""{"required":false}"""))
            val connection = ServerConnection.parse(server.url("/").toString(), true)
            val client = connection.newHttpClient(OriginCookieJar(MemoryCookieStorage(), connection)).newBuilder()
                .eventListener(object : okhttp3.EventListener() {
                    override fun responseHeadersEnd(call: okhttp3.Call, response: okhttp3.Response) { headersReceived.countDown() }
                }).build()
            val api = AnimeApi(client, connection)
            val job = launch(Dispatchers.IO) { api.setupStatus() }
            server.takeRequest()
            assertTrue(headersReceived.await(1, java.util.concurrent.TimeUnit.SECONDS))
            assertEquals(1, client.dispatcher.runningCallsCount())
            job.cancel()
            job.join()
            withTimeout(1_000) {
                while (client.dispatcher.runningCallsCount() != 0) kotlinx.coroutines.delay(5)
            }
            assertTrue(job.isCancelled)
            assertEquals(0, client.dispatcher.runningCallsCount())
        } finally { server.shutdown() }
    }

    @Test fun chunkedOversizedResponseIsRejected() = runBlocking {
        val server = MockWebServer().apply { start() }
        try {
            server.enqueue(MockResponse().setChunkedBody("x".repeat(1_048_577), 8192))
            val connection = ServerConnection.parse(server.url("/").toString(), true)
            val api = AnimeApi(connection.newHttpClient(OriginCookieJar(MemoryCookieStorage(), connection)), connection)
            try { api.setupStatus(); fail("chunked oversized response must be rejected") }
            catch (e: IllegalArgumentException) { assertEquals("Response is too large", e.message) }
        } finally { server.shutdown() }
    }

    @Test fun rateLimitAndChineseLegacyArrayAreNotSilentlyAccepted() = runBlocking {
        val server = MockWebServer().apply { start() }
        try {
            server.enqueue(MockResponse().setResponseCode(429))
            server.enqueue(MockResponse().setBody("""[{"id":1,"title":"番剧"}]"""))
            val connection = ServerConnection.parse(server.url("/").toString(), true)
            val api = AnimeApi(connection.newHttpClient(OriginCookieJar(MemoryCookieStorage(), connection)), connection)
            try { api.setupStatus(); fail("429 must be surfaced") }
            catch (e: ApiError) { assertEquals(429, e.status) }
            try { api.page(); fail("legacy array must not decode as paged response") }
            catch (_: kotlinx.serialization.SerializationException) { }
        } finally { server.shutdown() }
    }

    @Test fun lateLoginResponseAfterRetirementCannotPersistSession() = runBlocking {
        val server = MockWebServer().apply { start() }
        try {
            server.enqueue(MockResponse().setHeadersDelay(150, java.util.concurrent.TimeUnit.MILLISECONDS)
                .addHeader("Set-Cookie", "auto_anime_session=test-only-session; Path=/; HttpOnly")
                .setBody("""{"ok":true}"""))
            val connection = ServerConnection.parse(server.url("/").toString(), true)
            val storage = MemoryCookieStorage()
            val jar = OriginCookieJar(storage, connection, "test-account")
            val api = AnimeApi(connection.newHttpClient(jar), connection)
            val result = async(Dispatchers.IO) { api.login("test-user", "test-password") }
            server.takeRequest()
            jar.retire()
            assertTrue(result.await().ok)
            assertTrue(jar.activeCookies().isEmpty())
        } finally { server.shutdown() }
    }

    @Test fun redirectToAnotherOriginIsNotFollowedOrSentSessionCookie() = runBlocking {
        val server = MockWebServer().apply { start() }
        val other = MockWebServer().apply { start() }
        try {
            val connection = ServerConnection.parse(server.url("/proxy/").toString(), true)
            val jar = OriginCookieJar(MemoryCookieStorage(), connection, "test-account")
            jar.saveFromResponse(connection.base, listOf(okhttp3.Cookie.Builder().name("auto_anime_session").value("test-only").hostOnlyDomain(connection.base.host).path("/").build()))
            val api = AnimeApi(connection.newHttpClient(jar), connection)
            for (code in listOf(301, 302)) {
                server.enqueue(MockResponse().setResponseCode(code).addHeader("Location", other.url("/steal").toString()))
                try { api.setupStatus(); fail("redirect must be surfaced") }
                catch (e: ApiError) { assertEquals(code, e.status) }
                server.takeRequest()
            }
            assertEquals(0, other.requestCount)
        } finally { server.shutdown(); other.shutdown() }
    }
}

private class MemoryCookieStorage : CookieStorage {
    private val values = mutableMapOf<String, List<okhttp3.Cookie>>()
    override fun load(origin: String) { }
    override fun save(origin: String, cookies: List<okhttp3.Cookie>) { values[origin] = cookies }
    override fun cookies(origin: String) = values[origin].orEmpty()
    override fun clear(origin: String) { values.remove(origin) }
}
