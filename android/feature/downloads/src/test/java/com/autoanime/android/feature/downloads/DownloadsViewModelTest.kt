package com.autoanime.android.feature.downloads

import app.cash.turbine.test
import com.autoanime.android.core.data.ConnectionStore
import com.autoanime.android.core.data.SavedConnection
import com.autoanime.android.core.data.SessionRepository
import com.autoanime.android.core.data.SessionState
import com.autoanime.android.core.model.AppError
import com.autoanime.android.core.model.DownloadAction
import com.autoanime.android.core.model.DownloadItem
import com.autoanime.android.core.model.DownloadPage
import com.autoanime.android.core.model.RuntimeStatus
import com.autoanime.android.core.model.SetupState
import com.autoanime.android.core.model.StatusGroup
import com.autoanime.android.core.network.AnimeApi
import com.autoanime.android.core.network.ApiError
import com.autoanime.android.core.network.CookieStorage
import com.autoanime.android.core.network.ServerConnection
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import okhttp3.Cookie
import okhttp3.OkHttpClient
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class DownloadsViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val scope = TestScope(dispatcher)
    private val apis = mutableListOf<FakeApi>()
    private val sessions = SessionRepository(
        cookieStorage = { MemoryCookieStorage() },
        connectionStore = NoopConnectionStore,
        apiFactory = { _, _ -> FakeApi().also { apis += it } },
        imageLoaderFactory = { error("covers are not loaded in unit tests") },
        scope = scope.backgroundScope,
        io = dispatcher,
    )

    @Before fun setUp() = Dispatchers.setMain(dispatcher)
    @After fun tearDown() = Dispatchers.resetMain()

    @Test fun pageForPreviousFilterCannotReplaceNewFilter() = scope.runTest {
        val api = connect()
        val all = CompletableDeferred<DownloadPage>()
        api.pages = { group, _, _ -> if (group == null) all.await() else itemsPage(item(9, "已暂停")) }
        val model = DownloadsViewModel(sessions, dispatcher)
        runCurrent()
        model.selectGroup(StatusGroup.Paused)
        runCurrent()
        all.complete(itemsPage(item(1, "已保存")))
        runCurrent()
        assertEquals(listOf(9L), model.state.value.items.map { it.item.id })
        assertEquals(StatusGroup.Paused, model.state.value.group)
    }

    @Test fun searchIsDebouncedAndOnlyTheLastQueryIsSent() = scope.runTest {
        val api = connect()
        api.pages = { _, query, _ -> itemsPage(item(if (query == null) 1 else 2, "已保存")) }
        val model = DownloadsViewModel(sessions, dispatcher)
        runCurrent()
        api.calls.clear()
        model.setQuery("葬")
        advanceTimeBy(100)
        model.setQuery("葬送")
        advanceTimeBy(DownloadsViewModel.SEARCH_DEBOUNCE_MS + 1)
        runCurrent()
        assertEquals(listOf("葬送"), api.calls.map { it.query })
    }

    @Test fun refreshReplacesTheLoadedRangeAndDropsRemovedRows() = scope.runTest {
        val api = connect()
        var rows = listOf(item(5), item(4), item(3), item(2))
        api.pages = { _, _, before -> pageOf(rows, before, size = 2) }
        val model = DownloadsViewModel(sessions, dispatcher)
        runCurrent()
        model.loadMore()
        runCurrent()
        assertEquals(listOf(5L, 4L, 3L, 2L), model.state.value.items.map { it.item.id })

        rows = listOf(item(6), item(5, "已保存"), item(4), item(2))
        model.refresh()
        runCurrent()
        val state = model.state.value
        assertEquals(listOf(6L, 5L, 4L, 2L), state.items.map { it.item.id })
        assertEquals("已保存", state.items[1].item.status)
        assertTrue(state.endReached)
    }

    @Test fun pollsOnlyWhileVisibleAndTasksAreActive() = scope.runTest {
        val api = connect()
        api.counts = mapOf("下载中" to 1)
        api.pages = { _, _, _ -> itemsPage(item(1, "下载中")) }
        val model = DownloadsViewModel(sessions, dispatcher)
        model.setVisible(true)
        runCurrent()
        api.calls.clear()
        advanceTimeBy(DownloadsViewModel.POLL_INTERVAL_MS + 1)
        runCurrent()
        assertEquals(1, api.calls.size)

        api.counts = mapOf("已保存" to 1)
        advanceTimeBy(DownloadsViewModel.POLL_INTERVAL_MS + 1)
        runCurrent()
        api.calls.clear()
        advanceTimeBy(DownloadsViewModel.POLL_INTERVAL_MS * 5)
        runCurrent()
        assertTrue("no active work, so polling stops", api.calls.isEmpty())

        api.counts = mapOf("下载中" to 1)
        model.refresh()
        runCurrent()
        model.setVisible(false)
        api.calls.clear()
        advanceTimeBy(DownloadsViewModel.POLL_INTERVAL_MS * 5)
        runCurrent()
        assertTrue("hidden screens do not poll", api.calls.isEmpty())
    }

    @Test fun rejectedActionShowsServerReasonAndRefreshes() = scope.runTest {
        val api = connect()
        api.pages = { _, _, _ -> itemsPage(item(1, "下载中")) }
        val gate = CompletableDeferred<Boolean>()
        api.action = { _, _ -> if (gate.await()) true else throw ApiError(409, "Request failed", "只有已暂停的任务可以继续") }
        val model = DownloadsViewModel(sessions, dispatcher)
        runCurrent()
        val row = model.state.value.items.single()
        model.messages.test {
            model.perform(row, DownloadAction.Resume)
            model.perform(row, DownloadAction.Resume)
            runCurrent()
            assertEquals(setOf(1L), model.state.value.busy)
            api.calls.clear()
            gate.complete(false)
            runCurrent()
            assertEquals(DownloadMessage.Failed(AppError.Rejected(409, "只有已暂停的任务可以继续")), awaitItem())
            assertTrue(model.state.value.busy.isEmpty())
            assertEquals(1, api.actions.size)
            assertEquals(1, api.calls.size)
        }
    }

    @Test fun unauthorizedActionEndsTheSession() = scope.runTest {
        val api = connect()
        api.pages = { _, _, _ -> itemsPage(item(1, "已暂停")) }
        api.action = { _, _ -> throw ApiError(401, "Session expired") }
        val model = DownloadsViewModel(sessions, dispatcher)
        runCurrent()
        model.perform(model.state.value.items.single(), DownloadAction.Resume)
        runCurrent()
        assertEquals(AppError.SessionExpired, (sessions.state.value as SessionState.SignedOut).error)
        assertTrue(model.state.value.items.isEmpty())
    }

    @Test fun failedBackgroundUpdateKeepsRowsAndMarksThemStale() = scope.runTest {
        val api = connect()
        api.counts = mapOf("下载中" to 1)
        api.pages = { _, _, _ -> itemsPage(item(1, "下载中")) }
        val model = DownloadsViewModel(sessions, dispatcher)
        model.setVisible(true)
        runCurrent()
        api.pages = { _, _, _ -> throw java.io.IOException("offline") }
        advanceTimeBy(DownloadsViewModel.POLL_INTERVAL_MS + 1)
        runCurrent()
        assertTrue(model.state.value.stale)
        assertEquals(listOf(1L), model.state.value.items.map { it.item.id })
        assertFalse(model.state.value.error != null)
        model.setVisible(false)
    }

    private fun TestScope.connect(): FakeApi {
        sessions.connect(ServerConnection.parse("https://nas.example/", false), "", "")
        runCurrent()
        apis.last().setup.complete(SetupState(false))
        runCurrent()
        return apis.last()
    }
}

private fun item(id: Long, status: String = "待下载") = DownloadItem(id = id, title = "item $id", status = status)

private fun itemsPage(vararg items: DownloadItem) = DownloadPage(items.toList(), next_before = 0)

/** Serves [rows] (newest first) like the server: ids below [before], [size] per page. */
private fun pageOf(rows: List<DownloadItem>, before: Long?, size: Int): DownloadPage {
    val remaining = rows.filter { before == null || it.id < before }
    val chunk = remaining.take(size)
    return DownloadPage(chunk, next_before = if (remaining.size > size) chunk.last().id else 0)
}

private data class ItemsCall(val group: String?, val query: String?, val before: Long?)

private class FakeApi : AnimeApi(OkHttpClient(), ServerConnection.parse("https://nas.example/", false)) {
    val setup = CompletableDeferred<SetupState>()
    val calls = mutableListOf<ItemsCall>()
    val actions = mutableListOf<Pair<Long, DownloadAction>>()
    var counts: Map<String, Int> = emptyMap()
    var pages: suspend (String?, String?, Long?) -> DownloadPage = { _, _, _ -> itemsPage() }
    var action: suspend (Long, DownloadAction) -> Boolean = { _, _ -> true }
    override suspend fun setupStatus() = setup.await()
    override suspend fun probeSession() = Unit
    override suspend fun runtimeStatus() = RuntimeStatus(counts)
    override suspend fun items(group: String?, query: String?, before: Long?): DownloadPage {
        calls += ItemsCall(group, query, before)
        return pages(group, query, before)
    }
    override suspend fun itemAction(id: Long, action: DownloadAction): Boolean {
        actions += id to action
        return this.action(id, action)
    }
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
