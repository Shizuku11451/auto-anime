package com.autoanime.android

import androidx.test.core.app.ActivityScenario
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
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import com.autoanime.android.core.ServerConnection
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import org.junit.runner.RunWith
import java.util.concurrent.TimeUnit
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

    private fun fixtureDispatcher(streamUnauthorized: Boolean, libraryUnauthorized: Boolean = false) = object : Dispatcher() {
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
            "/prefix/api/items/77/stream" -> if (hasSession(request) && streamUnauthorized) {
                MockResponse().setResponseCode(401)
            } else MockResponse().setResponseCode(404)
            "/prefix/api/logout" -> MockResponse()
                .addHeader("Set-Cookie", "auto_anime_session=; Path=/; Max-Age=0; HttpOnly")
                .setBody("""{"ok":true}""")
            else -> MockResponse().setResponseCode(404)
        }

        private fun hasSession(request: RecordedRequest) =
            request.getHeader("Cookie")?.contains("auto_anime_session=instrumented-session") == true
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
        assertDestination(scenario, destinationId)
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
