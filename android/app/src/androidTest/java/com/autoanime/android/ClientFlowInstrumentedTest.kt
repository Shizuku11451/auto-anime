package com.autoanime.android

import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.state.ToggleableState
import androidx.compose.ui.test.ExperimentalTestApi
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isRoot
import androidx.compose.ui.test.printToString
import androidx.compose.ui.test.junit4.createEmptyComposeRule
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performSemanticsAction
import androidx.compose.ui.test.performTextReplacement
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.autoanime.android.core.network.ServerConnection
import com.autoanime.android.feature.connect.ConnectTestTags
import com.autoanime.android.feature.downloads.DownloadsTestTags
import com.autoanime.android.navigation.NavTestTags
import com.autoanime.android.feature.player.PlayerInstances
import com.autoanime.android.feature.player.PlayerSemantics
import com.autoanime.android.feature.player.PlayerTestTags
import com.autoanime.android.feature.settings.SettingsTestTags
import dagger.hilt.android.EntryPointAccessors
import kotlinx.coroutines.runBlocking
import okhttp3.Request
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import java.util.concurrent.TimeUnit
import com.autoanime.android.core.designsystem.R as DesignR
import com.autoanime.android.feature.downloads.R as DownloadsR
import com.autoanime.android.feature.settings.R as SettingsR

@OptIn(ExperimentalTestApi::class)
@RunWith(AndroidJUnit4::class)
class ClientFlowInstrumentedTest {
    @get:Rule val compose = createEmptyComposeRule()

    private val context get() = InstrumentationRegistry.getInstrumentation().targetContext
    private val app get() = EntryPointAccessors.fromApplication(context, DebugEntryPoint::class.java)
    private val server = MockWebServer()
    private var ownsPersistedTestConnection = false
    private val png1x1 = byteArrayOf(
        0x89.toByte(), 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
        0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
        0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
        0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4.toByte(),
        0x89.toByte(), 0x00, 0x00, 0x00, 0x0b, 0x49, 0x44, 0x41,
        0x54, 0x78, 0x9c.toByte(), 0x63, 0x60, 0x00, 0x02, 0x00,
        0x00, 0x05, 0x00, 0x01, 0xa5.toByte(), 0xf6.toByte(), 0x45, 0x40,
        0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae.toByte(), 0x42, 0x60, 0x82.toByte(),
    )
    private val media by lazy {
        InstrumentationRegistry.getInstrumentation().context.assets.open("fixture.mp4").use { it.readBytes() }
    }
    private val serverUrl get() = "http://127.0.0.1:${server.port}/prefix/"
    /** Server-side state of the synthetic download task; null once its record is removed. */
    @Volatile private var taskStatus: String? = "下载中"

    @After fun tearDown() {
        clearTestConnection()
        server.shutdown()
    }

    @Test fun playableFixtureRendersSeeksAndReleasesOnExit() = launch(playableFixture = true) {
        connect()
        waitForPath("/prefix/api/animes?page=1&limit=30")
        compose.onNode(hasText("Fixture Anime") and hasClickAction()).performClick()
        compose.waitUntilExactlyOneExists(hasText("Play fixture"), TIMEOUT)
        compose.onNode(hasText("Play fixture") and hasClickAction()).performClick()
        compose.waitUntilExactlyOneExists(SemanticsMatcher.expectValue(PlayerSemantics.FirstFrameRendered, true), 10_000)

        val stream = nextMediaRequest() ?: throw AssertionError("No authenticated stream request")
        assertEquals("/prefix/api/items/77/stream", stream.path)
        assertEquals("auto_anime_session=instrumented-session", stream.getHeader("Cookie"))
        assertEquals("GET", stream.method)

        val connection = ServerConnection.parse(serverUrl, allowHttp = true)
        val head = rangeCall(connection, "HEAD", includeSession = true, range = false).use { response ->
            assertEquals(200, response.code)
            response.header("Content-Length")
        }
        assertEquals(media.size.toString(), head)
        val ranged = rangeCall(connection, "GET", includeSession = true).use { response ->
            assertEquals(206, response.code)
            assertEquals("bytes 100-199/${media.size}", response.header("Content-Range"))
            response.body.bytes()
        }
        assertEquals(100, ranged.size)
        assertEquals(401, rangeCall(connection, "GET", includeSession = false).use { it.code })

        // Seek through the real progress slider, then close with the on-screen button.
        showPlayerControls()
        compose.onNode(hasTestTag(PlayerTestTags.SEEK_BAR)).performSemanticsAction(SemanticsActions.SetProgress) { it(0.6f) }
        compose.waitUntil(TIMEOUT) {
            compose.onAllNodes(SemanticsMatcher("seeked past 2 s") { node ->
                node.config.getOrElseNullable(PlayerSemantics.LastSeekPositionMs) { null }?.let { it >= 2_000 } == true
            }).fetchSemanticsNodes().isNotEmpty()
        }
        showPlayerControls()
        compose.onNode(hasTestTag(PlayerTestTags.CLOSE)).performClick()
        compose.waitUntilExactlyOneExists(hasText("Play fixture"), TIMEOUT)
        compose.waitUntil(TIMEOUT) { PlayerInstances.active.get() == 0 }
    }

    @Test fun loginLibraryDetailCoverAndLogoutUsePrefixedAuthenticatedEndpoints() = launch {
        connect()
        val page = waitForPath("/prefix/api/animes?page=1&limit=30")
        assertEquals("auto_anime_session=instrumented-session", page.getHeader("Cookie"))
        compose.onNode(hasText("Fixture Anime") and hasClickAction()).performClick()
        compose.waitUntilExactlyOneExists(hasText("Fixture summary"), TIMEOUT)
        val cover = waitForPath("/prefix/api/covers/cover.png")
        assertEquals("auto_anime_session=instrumented-session", cover.getHeader("Cookie"))

        compose.onNode(hasText(context.getString(SettingsR.string.settings)) and hasClickAction()).performClick()
        compose.waitUntilExactlyOneExists(hasTestTag(SettingsTestTags.LOGOUT), TIMEOUT)
        compose.onNode(hasTestTag(SettingsTestTags.LOGOUT)).performClick()
        compose.waitUntilExactlyOneExists(hasTestTag(ConnectTestTags.CONNECT), TIMEOUT)
        val logout = waitForPath("/prefix/api/logout")
        assertEquals("auto_anime_session=instrumented-session", logout.getHeader("Cookie"))
    }

    @Test fun downloadsPauseAndRemoveTargetOneItemAndShowServerState() = launch {
        connect()
        compose.onNode(hasTestTag(NavTestTags.DOWNLOADS)).performClick()
        compose.waitUntilExactlyOneExists(hasTestTag(DownloadsTestTags.row(88)), TIMEOUT)
        val list = waitForPath("/prefix/api/items?page=1")
        assertEquals("auto_anime_session=instrumented-session", list.getHeader("Cookie"))

        compose.onNode(hasTestTag(DownloadsTestTags.PAUSE) and hasAnyAncestor(hasTestTag(DownloadsTestTags.row(88)))).performClick()
        val pause = waitForPath("/prefix/api/items/88/pause")
        assertEquals("POST", pause.method)
        assertEquals("auto_anime_session=instrumented-session", pause.getHeader("Cookie"))
        // The row shows what the server recorded after the action, not an optimistic guess.
        compose.waitUntilExactlyOneExists(
            hasTestTag(DownloadsTestTags.status(88)) and hasText(context.getString(DownloadsR.string.group_paused)),
            TIMEOUT,
        )

        compose.onNode(hasTestTag(DownloadsTestTags.REMOVE) and hasAnyAncestor(hasTestTag(DownloadsTestTags.row(88)))).performClick()
        compose.waitUntilExactlyOneExists(hasTestTag(DownloadsTestTags.CONFIRM_REMOVE), TIMEOUT)
        compose.onNode(hasTestTag(DownloadsTestTags.CONFIRM_REMOVE)).performClick()
        val remove = waitForPath("/prefix/api/items/88")
        assertEquals("DELETE", remove.method)
        compose.waitUntilExactlyOneExists(hasText(context.getString(DownloadsR.string.downloads_empty)), TIMEOUT)
    }

    @Test fun streamUnauthorizedResponseReturnsFromPlayerToConnect() = launch(streamUnauthorized = true) {
        connect()
        compose.onNode(hasText("Fixture Anime") and hasClickAction()).performClick()
        compose.waitUntilExactlyOneExists(hasText("Play fixture"), TIMEOUT)
        compose.onNode(hasText("Play fixture") and hasClickAction()).performClick()
        // Blocking on the server does not advance Compose frames; let the player screen compose first.
        compose.waitUntilAtLeastOneExists(hasTestTag(PlayerTestTags.PLAYER) or hasTestTag(ConnectTestTags.CONNECT), TIMEOUT)
        val stream = waitForPath("/prefix/api/items/77/stream")
        assertEquals("auto_anime_session=instrumented-session", stream.getHeader("Cookie"))
        compose.waitUntilExactlyOneExists(hasTestTag(ConnectTestTags.CONNECT), TIMEOUT)
        compose.waitUntilExactlyOneExists(hasText(context.getString(DesignR.string.error_session_expired)), TIMEOUT)
    }

    @Test fun unauthorizedLibraryResponseReturnsToConnect() = launch(libraryUnauthorized = true) {
        connect(expectLibrary = false)
        // The page request is only sent once the library composes, so wait on the UI first.
        compose.waitUntilExactlyOneExists(hasText(context.getString(DesignR.string.error_session_expired)), TIMEOUT)
        compose.waitUntilExactlyOneExists(hasTestTag(ConnectTestTags.CONNECT), TIMEOUT)
        val page = waitForPath("/prefix/api/animes?page=1&limit=30")
        assertEquals("auto_anime_session=instrumented-session", page.getHeader("Cookie"))
    }

    private fun launch(
        streamUnauthorized: Boolean = false,
        libraryUnauthorized: Boolean = false,
        playableFixture: Boolean = false,
        body: () -> Unit,
    ) {
        server.dispatcher = fixtureDispatcher(streamUnauthorized, libraryUnauthorized, playableFixture)
        server.start()
        assertFreshTestApp()
        ActivityScenario.launch(MainActivity::class.java).use { body() }
    }

    private fun connect(expectLibrary: Boolean = true) {
        compose.waitUntilExactlyOneExists(hasTestTag(ConnectTestTags.SERVER_URL), TIMEOUT)
        // Toggle before typing: the keyboard animation shifts the layout under injected taps.
        val allowHttp = compose.onNode(hasTestTag(ConnectTestTags.ALLOW_HTTP))
        if (allowHttp.fetchSemanticsNode().config[SemanticsProperties.ToggleableState] != ToggleableState.On) allowHttp.performClick()
        compose.onNode(hasTestTag(ConnectTestTags.SERVER_URL)).performTextReplacement(serverUrl)
        compose.onNode(hasTestTag(ConnectTestTags.USERNAME)).performTextReplacement("test-user")
        compose.onNode(hasTestTag(ConnectTestTags.PASSWORD)).performTextReplacement("test-password")
        compose.onNode(hasTestTag(ConnectTestTags.CONNECT)).performClick()
        val setup = runCatching { waitForPath("/prefix/api/setup/status") }.getOrElse {
            throw AssertionError("${it.message}; screen:" + compose.onAllNodes(isRoot())[0].printToString(), it)
        }
        assertEquals("/prefix/api/setup/status", setup.path)
        val login = waitForPath("/prefix/api/login")
        assertEquals("{\"username\":\"test-user\",\"password\":\"test-password\"}", login.body.readUtf8())
        if (expectLibrary) compose.waitUntilExactlyOneExists(hasText("Fixture Anime"), TIMEOUT)
    }

    /** Controls auto-hide while playing; a tap on the video brings them back. */
    private fun showPlayerControls() {
        if (compose.onAllNodes(hasTestTag(PlayerTestTags.CLOSE)).fetchSemanticsNodes().isEmpty()) {
            compose.onNode(hasTestTag(PlayerTestTags.PLAYER)).performClick()
        }
        compose.waitUntilExactlyOneExists(hasTestTag(PlayerTestTags.CLOSE), TIMEOUT)
    }

    private fun fixtureDispatcher(streamUnauthorized: Boolean, libraryUnauthorized: Boolean, playableFixture: Boolean) = object : Dispatcher() {
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
            else -> downloadsResponse(request)
        }

        private fun downloadsResponse(request: RecordedRequest): MockResponse {
            if (!hasSession(request)) return MockResponse().setResponseCode(401)
            val status = taskStatus
            return when {
                request.path == "/prefix/api/status" ->
                    MockResponse().setBody(if (status == null) "{\"counts\":{}}" else "{\"counts\":{\"$status\":1}}")
                request.path == "/prefix/api/items?page=1" -> MockResponse().setBody(
                    if (status == null) "{\"items\":[],\"next_before\":0}"
                    else "{\"items\":[{\"id\":88,\"title\":\"[Group] Fixture 01\",\"status\":\"$status\",\"anime_title\":\"Fixture Anime\"," +
                        "\"episode\":\"1\",\"progress\":40,\"download_speed\":2048,\"link\":\"magnet:?xt=urn:btih:fixture\"}],\"next_before\":0}",
                )
                request.path == "/prefix/api/items/88/pause" && request.method == "POST" -> {
                    taskStatus = "已暂停"
                    MockResponse().setBody("{\"ok\":true,\"changed\":true}")
                }
                request.path == "/prefix/api/items/88" && request.method == "DELETE" -> {
                    taskStatus = null
                    MockResponse().setBody("{\"ok\":true}")
                }
                else -> MockResponse().setResponseCode(404)
            }
        }

        private fun hasSession(request: RecordedRequest) =
            request.getHeader("Cookie")?.contains("auto_anime_session=instrumented-session") == true
    }

    private fun mediaResponse(request: RecordedRequest): MockResponse {
        val response = MockResponse().addHeader("Content-Type", "video/mp4").addHeader("Accept-Ranges", "bytes")
        if (request.method == "HEAD") return response.setBody(okio.Buffer()).setHeader("Content-Length", media.size)
        val range = request.getHeader("Range")?.removePrefix("bytes=") ?: return response.setBody(okio.Buffer().write(media))
        val start = range.substringBefore('-').toLongOrNull() ?: 0L
        if (start >= media.size) return response.setResponseCode(416).addHeader("Content-Range", "bytes */${media.size}")
        val end = range.substringAfter('-', "").toLongOrNull()?.coerceAtMost(media.size.toLong() - 1) ?: (media.size - 1).toLong()
        val body = media.copyOfRange(start.toInt(), end.toInt() + 1)
        return response.setResponseCode(206).addHeader("Content-Range", "bytes $start-$end/${media.size}").setBody(okio.Buffer().write(body))
    }

    private fun rangeCall(connection: ServerConnection, method: String, includeSession: Boolean, range: Boolean = true): okhttp3.Response {
        val request = Request.Builder().url(connection.resolve("api/items/77/stream"))
            .apply { if (includeSession) header("Cookie", "auto_anime_session=instrumented-session") }
        if (range) request.header("Range", "bytes=100-199")
        if (method == "HEAD") request.head()
        return connection.newHttpClient(okhttp3.CookieJar.NO_COOKIES).newCall(request.build()).execute()
    }

    private fun nextMediaRequest(): RecordedRequest? {
        repeat(20) {
            val request = server.takeRequest(500, TimeUnit.MILLISECONDS) ?: return@repeat
            if (request.path == "/prefix/api/items/77/stream") return request
        }
        return null
    }

    private fun waitForPath(path: String): RecordedRequest {
        val seen = mutableListOf<String?>()
        repeat(30) {
            val request = server.takeRequest(1, TimeUnit.SECONDS) ?: return@repeat
            if (request.path == path) return request
            seen += request.path
        }
        throw AssertionError("No request received for $path; saw $seen")
    }

    private fun assertFreshTestApp() {
        assertNull("E2E requires a disposable test app with no persisted login", runBlocking { app.connectionStore().load() })
        ownsPersistedTestConnection = true
    }

    /** Only clears what this test created: the synthetic account's cookie and the saved connection. */
    private fun clearTestConnection() {
        if (!ownsPersistedTestConnection) return
        app.sessions().logout()
        val base = ServerConnection.parse(serverUrl, allowHttp = true).base
        val origin = "${base.scheme}://${base.host}:${base.port}"
        app.cookieStorage().clear("${origin.length}:$origin${"test-user".length}:test-user")
        runBlocking { app.connectionStore().clear() }
        ownsPersistedTestConnection = false
    }

    private companion object {
        const val TIMEOUT = 5_000L
    }
}
