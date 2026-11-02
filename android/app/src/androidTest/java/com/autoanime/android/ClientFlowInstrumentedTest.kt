package com.autoanime.android

import androidx.test.core.app.ActivityScenario
import androidx.test.espresso.NoMatchingViewException
import androidx.test.espresso.Espresso.onView
import androidx.test.espresso.action.ViewActions.click
import androidx.test.espresso.action.ViewActions.replaceText
import androidx.test.espresso.action.ViewActions.closeSoftKeyboard
import androidx.test.espresso.assertion.ViewAssertions.matches
import androidx.test.espresso.matcher.ViewMatchers.isDisplayed
import androidx.test.espresso.matcher.ViewMatchers.withId
import androidx.test.espresso.matcher.ViewMatchers.withText
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.media3.common.Player
import androidx.media3.common.Player.DISCONTINUITY_REASON_SEEK
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import okhttp3.Request
import com.autoanime.android.core.ServerConnection
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import org.junit.runner.RunWith
import java.util.concurrent.TimeUnit
import java.util.concurrent.CountDownLatch
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicReference
import kotlinx.coroutines.runBlocking

@RunWith(AndroidJUnit4::class)
class ClientFlowInstrumentedTest {
    private var ownsPersistedTestConnection = false
    private var testServerUrl: String? = null
    private val png1x1 = byteArrayOf(
        0x89.toByte(), 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
        0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
        0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
        0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4.toByte(),
        0x89.toByte(), 0x00, 0x00, 0x00, 0x0b, 0x49, 0x44, 0x41,
        0x54, 0x78, 0x9c.toByte(), 0x63, 0x60, 0x00, 0x02, 0x00,
        0x00, 0x05, 0x00, 0x01, 0xa5.toByte(), 0xf6.toByte(), 0x45, 0x40,
        0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae.toByte(), 0x42, 0x60, 0x82.toByte()
    )
    private val media by lazy { InstrumentationRegistry.getInstrumentation().context.assets.open("fixture.mp4").use { it.readBytes() } }
    @Test fun playableFixtureRendersSeeksAndReleasesOnExit() {
        val server = MockWebServer()
        server.dispatcher = fixtureDispatcher(streamUnauthorized = false, playableFixture = true)
        server.start()
        try {
            assertFreshTestApp()
            ActivityScenario.launch(MainActivity::class.java).use { scenario ->
                connect(server)
                waitForPath(server, "/prefix/api/animes?page=1&limit=30")
                onView(withText("Fixture Anime")).perform(click())
                awaitDisplayed(withText("Play fixture"))
                val playerRef = AtomicReference<Player>()
                val playerViewRef = AtomicReference<androidx.media3.ui.PlayerView>()
                val rendered = CountDownLatch(1)
                val sought = CountDownLatch(1)
                val released = AtomicBoolean(false)
                scenario.onActivity { activity ->
                    val host = activity.supportFragmentManager.findFragmentById(R.id.nav_host) as androidx.navigation.fragment.NavHostFragment
                    host.childFragmentManager.registerFragmentLifecycleCallbacks(object : androidx.fragment.app.FragmentManager.FragmentLifecycleCallbacks() {
                        override fun onFragmentViewCreated(fm: androidx.fragment.app.FragmentManager, f: androidx.fragment.app.Fragment, v: android.view.View, savedInstanceState: android.os.Bundle?) {
                            if (f !is com.autoanime.android.feature.PlayerFragment) return
                            val playerView = v.findViewById<androidx.media3.ui.PlayerView>(R.id.player_view)
                            playerViewRef.set(playerView)
                            val player = playerView.player ?: throw AssertionError("PlayerView has no active player")
                            playerRef.set(player)
                            player.addListener(object : Player.Listener {
                                override fun onRenderedFirstFrame() { rendered.countDown() }
                                override fun onPositionDiscontinuity(oldPosition: Player.PositionInfo, newPosition: Player.PositionInfo, reason: Int) {
                                    if (reason == DISCONTINUITY_REASON_SEEK) sought.countDown()
                                }
                            })
                        }
                    }, true)
                }
                onView(withText("Play fixture")).perform(click())
                awaitDisplayed(withId(R.id.player_view))
                assertTrue("Expected Media3 to render a video frame", rendered.await(10, TimeUnit.SECONDS))

                val stream = nextMediaRequest(server) ?: throw AssertionError("No authenticated stream request")
                assertEquals("/prefix/api/items/77/stream", stream.path)
                assertEquals("auto_anime_session=instrumented-session", stream.getHeader("Cookie"))
                assertEquals("GET", stream.method)

                val serverConnection = ServerConnection.parse("http://127.0.0.1:${server.port}/prefix/", allowHttp = true)
                val head = rangeCall(serverConnection, "HEAD", includeSession = true, range = false).use { response ->
                    assertEquals(200, response.code)
                    response.header("Content-Length")
                }
                assertEquals(media.size.toString(), head)
                val rangeResponse = rangeCall(serverConnection, "GET", includeSession = true).use { response ->
                    assertEquals(206, response.code)
                    assertEquals("bytes 100-199/${media.size}", response.header("Content-Range"))
                    response.body.bytes()
                }
                assertEquals(100, rangeResponse.size)
                val unauthorized = rangeCall(serverConnection, "GET", includeSession = false).use { it.code }
                assertEquals(401, unauthorized)

                scenario.onActivity { playerRef.get().pause() }
                val before = AtomicReference(0L)
                scenario.onActivity { before.set(playerRef.get().currentPosition) }
                scenario.onActivity { playerRef.get().seekTo(2500) }
                assertTrue("Expected Media3 seek discontinuity", sought.await(2, TimeUnit.SECONDS))
                val after = AtomicReference(0L)
                scenario.onActivity { after.set(playerRef.get().currentPosition) }
                assertTrue("Seek target should change playback position: before=${before.get()} after=${after.get()}", after.get() >= 2000L)

                val close = onView(withId(R.id.close)).check(matches(isDisplayed()))
                scenario.onActivity { activity ->
                    val button = activity.findViewById<android.view.View>(R.id.close)
                    val visible = android.graphics.Rect()
                    assertTrue("Close button must have a tappable area above system navigation bars", button.getGlobalVisibleRect(visible) && visible.height() >= button.height - 2)
                }
                close.perform(click())
                waitForDestination(scenario, R.id.detailFragment)
                scenario.onActivity { released.set(playerViewRef.get().player == null) }
                assertTrue("Closed player view should detach its released player", released.get())
            }
        } finally {
            clearTestConnection()
            server.shutdown()
        }
    }
    @Test fun loginLibraryDetailCoverAndLogoutUsePrefixedAuthenticatedEndpoints() {
        val server = MockWebServer()
        server.dispatcher = fixtureDispatcher(streamUnauthorized = false)
        server.start()
        try {
            assertFreshTestApp()
            ActivityScenario.launch(MainActivity::class.java).use { scenario ->
                connect(server)
                awaitDisplayed(withText("Fixture Anime"))
                val page = waitForPath(server, "/prefix/api/animes?page=1&limit=30")
                assertEquals("auto_anime_session=instrumented-session", page.getHeader("Cookie"))
                onView(withText("Fixture Anime")).perform(click())
                awaitDisplayed(withText("Fixture summary"))
                val cover = waitForPath(server, "/prefix/api/covers/cover.png")
                assertEquals("auto_anime_session=instrumented-session", cover.getHeader("Cookie"))

                onView(withId(R.id.logout)).perform(click())
                onView(withId(R.id.connect)).check(matches(isDisplayed()))
                val logout = waitForPath(server, "/prefix/api/logout")
                assertEquals("auto_anime_session=instrumented-session", logout.getHeader("Cookie"))
                assertDestination(scenario, R.id.connectFragment)
            }
        } finally {
            clearTestConnection()
            server.shutdown()
        }
    }

    @Test fun streamUnauthorizedResponseReturnsFromPlayerToConnect() {
        val server = MockWebServer()
        server.dispatcher = fixtureDispatcher(streamUnauthorized = true)
        server.start()
        try {
            assertFreshTestApp()
            ActivityScenario.launch(MainActivity::class.java).use { scenario ->
                connect(server)
                awaitDisplayed(withText("Fixture Anime"))
                val page = waitForPath(server, "/prefix/api/animes?page=1&limit=30")
                assertEquals("auto_anime_session=instrumented-session", page.getHeader("Cookie"))
                onView(withText("Fixture Anime")).perform(click())
                awaitDisplayed(withText("Play fixture"))
                onView(withText("Play fixture")).perform(click())
                val stream = waitForPath(server, "/prefix/api/items/77/stream")
                assertEquals("auto_anime_session=instrumented-session", stream.getHeader("Cookie"))
                waitForDestination(scenario, R.id.connectFragment)
            }
        } finally {
            clearTestConnection()
            server.shutdown()
        }
    }

    @Test fun unauthorizedLibraryResponseReturnsToConnect() {
        val server = MockWebServer()
        server.dispatcher = fixtureDispatcher(streamUnauthorized = false, libraryUnauthorized = true)
        server.start()
        try {
            assertFreshTestApp()
            ActivityScenario.launch(MainActivity::class.java).use { scenario ->
                connect(server, expectLibrary = false)
                val page = waitForPath(server, "/prefix/api/animes?page=1&limit=30")
                assertEquals("auto_anime_session=instrumented-session", page.getHeader("Cookie"))
                waitForDestination(scenario, R.id.connectFragment)
                awaitDisplayed(withId(R.id.connect))
                awaitDisplayed(withText("Session expired. Log in again."))
            }
        } finally {
            clearTestConnection()
            server.shutdown()
        }
    }

    private fun connect(server: MockWebServer, expectLibrary: Boolean = true) {
        val emulatorUrl = "http://127.0.0.1:${server.port}/prefix/"
        testServerUrl = emulatorUrl
        onView(withId(R.id.server_url)).perform(replaceText(emulatorUrl), closeSoftKeyboard())
        onView(withId(R.id.allow_http)).perform(click())
        onView(withId(R.id.username)).perform(replaceText("test-user"), closeSoftKeyboard())
        onView(withId(R.id.password)).perform(replaceText("test-password"), closeSoftKeyboard())
        onView(withId(R.id.connect)).perform(click())
        assertEquals("/prefix/api/setup/status", waitForPath(server, "/prefix/api/setup/status").path)
        val login = waitForPath(server, "/prefix/api/login")
        assertEquals("{\"username\":\"test-user\",\"password\":\"test-password\"}", login.body.readUtf8())
        if (expectLibrary) awaitDisplayed(withText("Fixture Anime"))
    }

    private fun fixtureDispatcher(streamUnauthorized: Boolean, libraryUnauthorized: Boolean = false, playableFixture: Boolean = false) = object : Dispatcher() {
        override fun dispatch(request: RecordedRequest): MockResponse = when (request.path) {
            "/prefix/api/setup/status" -> MockResponse().setBody("""{"required":false}""")
            "/prefix/api/login" -> MockResponse()
                .addHeader("Set-Cookie", "auto_anime_session=instrumented-session; Path=/; HttpOnly")
                .setBody("""{"ok":true}""")
            "/prefix/api/animes?page=1&limit=30" -> if (!libraryUnauthorized && hasSession(request)) {
                MockResponse().setBody("""{"items":[{"id":7,"title":"Fixture Anime","season":1}],"next_before":0}""")
            } else MockResponse().setResponseCode(401)
            "/prefix/api/animes/7" -> if (hasSession(request)) {
                MockResponse().setBody("""{"id":7,"title":"Fixture Anime","cover":"/api/covers/cover.png","summary":"Fixture summary","seasons":[{"season":1,"episodes":[{"episode":"1","items":[{"id":77,"title":"Play fixture","status":"已保存","episode":"1"}]}]}]}""")
            } else MockResponse().setResponseCode(401)
            "/prefix/api/covers/cover.png" -> if (hasSession(request)) {
                MockResponse().setHeader("Content-Type", "image/png").setBody(okio.Buffer().write(png1x1))
            } else MockResponse().setResponseCode(401)
            "/prefix/api/items/77/stream" -> when {
                streamUnauthorized && hasSession(request) -> MockResponse().setResponseCode(401)
                hasSession(request) && playableFixture -> mediaResponse(request)
                !hasSession(request) -> MockResponse().setResponseCode(401)
                else -> MockResponse().setResponseCode(404)
            }
            "/prefix/api/logout" -> MockResponse()
                .addHeader("Set-Cookie", "auto_anime_session=; Path=/; Max-Age=0; HttpOnly")
                .setBody("""{"ok":true}""")
            else -> MockResponse().setResponseCode(404)
        }

        private fun hasSession(request: RecordedRequest) =
            request.getHeader("Cookie")?.contains("auto_anime_session=instrumented-session") == true
    }

    private fun mediaResponse(request: RecordedRequest): MockResponse {
        val response = MockResponse().addHeader("Content-Type", "video/mp4").addHeader("Accept-Ranges", "bytes")
        if (request.method == "HEAD") return response.setBody(okio.Buffer()).setHeader("Content-Length", media.size)
        val range = request.getHeader("Range")?.removePrefix("bytes=")
        if (range == null) return response.setBody(okio.Buffer().write(media))
        val start = range.substringBefore('-').toLongOrNull() ?: 0L
        if (start >= media.size) return response.setResponseCode(416).addHeader("Content-Range", "bytes */${media.size}")
        val end = range.substringAfter('-', "").toLongOrNull()?.coerceAtMost(media.size.toLong() - 1) ?: (media.size - 1).toLong()
        val body = media.copyOfRange(start.toInt(), end.toInt() + 1)
        return response.setResponseCode(206).addHeader("Content-Range", "bytes $start-$end/${media.size}").setBody(okio.Buffer().write(body))
    }

    private fun rangeCall(server: ServerConnection, method: String, includeSession: Boolean, range: Boolean = true): okhttp3.Response {
        val request = Request.Builder().url(server.resolve("api/items/77/stream"))
            .apply { if (includeSession) header("Cookie", "auto_anime_session=instrumented-session") }
        if (range) request.header("Range", "bytes=100-199")
        if (method == "HEAD") request.head()
        return server.newHttpClient(okhttp3.CookieJar.NO_COOKIES).newCall(request.build()).execute()
    }

    private fun nextMediaRequest(server: MockWebServer): RecordedRequest? {
        repeat(20) {
            val request = server.takeRequest(500, TimeUnit.MILLISECONDS) ?: return@repeat
            if (request.path == "/prefix/api/items/77/stream") return request
        }
        return null
    }

    private fun waitForPath(server: MockWebServer, path: String): RecordedRequest {
        repeat(30) {
            val request = server.takeRequest(1, TimeUnit.SECONDS) ?: return@repeat
            if (request.path == path) return request
        }
        throw AssertionError("No request received for $path")
    }

    private fun awaitDisplayed(matcher: org.hamcrest.Matcher<android.view.View>) {
        repeat(50) {
            try {
                onView(matcher).check(matches(isDisplayed()))
                return
            } catch (_: AssertionError) {
                Thread.sleep(100)
            } catch (_: NoMatchingViewException) {
                Thread.sleep(100)
            }
        }
        onView(matcher).check(matches(isDisplayed()))
    }

    private fun assertFreshTestApp() {
        val app = InstrumentationRegistry.getInstrumentation().targetContext.applicationContext as AutoAnimeApp
        val saved = runBlocking { app.savedConnection() }
        assertNull("E2E requires a disposable test app with no persisted login", saved)
        ownsPersistedTestConnection = true
    }

    private fun clearTestConnection() {
        if (!ownsPersistedTestConnection) return
        val app = InstrumentationRegistry.getInstrumentation().targetContext.applicationContext as AutoAnimeApp
        app.clearSession()
        testServerUrl?.let { url ->
            val base = ServerConnection.parse(url, allowHttp = true).base
            val origin = "${base.scheme}://${base.host}:${base.port}"
            app.storage.clear("${origin.length}:$origin${"test-user".length}:test-user")
        }
        runBlocking { app.clearSavedConnection(app.generation.get()) }
        ownsPersistedTestConnection = false
        testServerUrl = null
    }

    private fun waitForDestination(scenario: ActivityScenario<MainActivity>, destinationId: Int) {
        repeat(50) {
            if (destination(scenario) == destinationId) return
            Thread.sleep(100)
        }
        scenario.onActivity { activity ->
            val nav = (activity.supportFragmentManager.findFragmentById(R.id.nav_host) as androidx.navigation.fragment.NavHostFragment).navController
            val actual = nav.currentDestination?.let { activity.resources.getResourceEntryName(it.id) }
            val previous = nav.previousBackStackEntry?.destination?.let { activity.resources.getResourceEntryName(it.id) }
            val playerFragment = (activity.supportFragmentManager.findFragmentById(R.id.nav_host) as androidx.navigation.fragment.NavHostFragment)
                .childFragmentManager.primaryNavigationFragment as? com.autoanime.android.feature.PlayerFragment
            val close = playerFragment?.requireView()?.findViewById<android.view.View>(R.id.close)
            fail("Navigation did not reach $destinationId; actual=$actual previous=$previous closeShown=${close?.isShown} closeEnabled=${close?.isEnabled} closeHasListener=${close?.hasOnClickListeners()}")
        }
    }

    private fun destination(scenario: ActivityScenario<MainActivity>): Int {
        var actual = 0
        scenario.onActivity { activity ->
            actual = (activity.supportFragmentManager.findFragmentById(R.id.nav_host) as androidx.navigation.fragment.NavHostFragment)
                .navController.currentDestination?.id ?: 0
        }
        return actual
    }

    private fun assertDestination(scenario: ActivityScenario<MainActivity>, destinationId: Int) {
        scenario.onActivity { activity ->
            val nav = (activity.supportFragmentManager.findFragmentById(R.id.nav_host) as androidx.navigation.fragment.NavHostFragment).navController
            assertEquals(destinationId, nav.currentDestination?.id)
        }
    }
}
