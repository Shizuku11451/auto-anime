package com.autoanime.android.feature.library

import com.autoanime.android.core.data.ConnectionStore
import com.autoanime.android.core.data.SavedConnection
import com.autoanime.android.core.data.SessionRepository
import com.autoanime.android.core.data.SessionState
import com.autoanime.android.core.model.Anime
import com.autoanime.android.core.model.AnimeDetail
import com.autoanime.android.core.model.AnimePage
import com.autoanime.android.core.model.AppError
import com.autoanime.android.core.model.ScheduleDay
import com.autoanime.android.core.model.SetupState
import com.autoanime.android.core.network.AnimeApi
import com.autoanime.android.core.network.ApiError
import com.autoanime.android.core.network.CookieStorage
import com.autoanime.android.core.network.ServerConnection
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import kotlinx.coroutines.withContext
import okhttp3.Cookie
import okhttp3.OkHttpClient
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.time.LocalDate

@OptIn(ExperimentalCoroutinesApi::class)
class LibraryViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val scope = TestScope(dispatcher)
    private val apis = mutableListOf<GatedApi>()
    private val sessions = SessionRepository(
        cookieStorage = { MemoryCookieStorage() },
        connectionStore = NoopConnectionStore,
        apiFactory = { _, _ -> GatedApi().also { apis += it } },
        imageLoaderFactory = { error("covers are not loaded in unit tests") },
        scope = scope.backgroundScope,
        io = dispatcher,
    )

    @Before fun setUp() = Dispatchers.setMain(dispatcher)
    @After fun tearDown() = Dispatchers.resetMain()

    @Test fun pageFromPreviousSessionCannotReplaceNewSession() = scope.runTest {
        val model = LibraryViewModel(sessions, dispatcher)
        connect("https://old.example/")
        val oldPage = apis.last().pages.single()
        connect("https://new.example/")
        apis.last().pages.single().complete(AnimePage(listOf(Anime(2, "new")), 0))
        runCurrent()
        oldPage.complete(AnimePage(listOf(Anime(1, "old")), 0))
        runCurrent()
        assertEquals(listOf(2L), model.state.value.items.map { it.id })
    }

    @Test fun loadMoreAppendsWithoutDuplicates() = scope.runTest {
        val model = LibraryViewModel(sessions, dispatcher)
        connect("https://nas.example/")
        val api = apis.last()
        api.pages.last().complete(AnimePage(listOf(Anime(3, "c"), Anime(2, "b")), next_before = 2))
        runCurrent()
        model.loadMore()
        runCurrent()
        api.pages.last().complete(AnimePage(listOf(Anime(2, "b"), Anime(1, "a")), next_before = 0))
        runCurrent()
        assertEquals(listOf(3L, 2L, 1L), model.state.value.items.map { it.id })
        assertTrue(model.state.value.endReached)
    }

    @Test fun unauthorizedPageEndsTheSession() = scope.runTest {
        val model = LibraryViewModel(sessions, dispatcher)
        connect("https://nas.example/")
        apis.last().pages.single().completeExceptionally(ApiError(401, "Session expired"))
        runCurrent()
        val state = sessions.state.value as SessionState.SignedOut
        assertEquals(AppError.SessionExpired, state.error)
        assertTrue(model.state.value.items.isEmpty())
    }

    @Test fun dayFilterSendsLocalDateAndIgnoresPagesForAnotherDay() = scope.runTest {
        val model = LibraryViewModel(sessions, dispatcher)
        model.today = { LocalDate.of(2026, 11, 3) }
        connect("https://nas.example/")
        val api = apis.last()
        val counts = listOf(1, 2, 3, 0, 0, 0, 0, 0, 4)
        api.pages.single().complete(AnimePage(listOf(Anime(1, "all")), 0, day_counts = counts))
        runCurrent()
        assertEquals(counts, model.state.value.dayCounts)
        assertEquals(2, model.state.value.todayWeekday)

        model.selectDay(2)
        runCurrent()
        val tuesday = api.pages.last()
        model.selectDay(ScheduleDay.FINISHED)
        runCurrent()
        assertEquals(PageCall(null, 2, "2026-11-03", true), api.pageCalls[1])
        assertEquals(PageCall(null, ScheduleDay.FINISHED, "2026-11-03", true), api.pageCalls[2])
        api.pages.last().complete(AnimePage(listOf(Anime(8, "finished")), 0, day_counts = counts))
        runCurrent()
        tuesday.complete(AnimePage(listOf(Anime(2, "tuesday")), 0, day_counts = counts))
        runCurrent()
        assertEquals(listOf(8L), model.state.value.items.map { it.id })
        assertEquals(ScheduleDay.FINISHED, model.state.value.day)
    }

    @Test fun serverWithoutDayCountsHidesTheFilter() = scope.runTest {
        val model = LibraryViewModel(sessions, dispatcher)
        connect("https://nas.example/")
        apis.last().pages.single().complete(AnimePage(listOf(Anime(1, "a")), 0))
        runCurrent()
        assertNull(model.state.value.dayCounts)
    }

    @Test fun lateDetailResponseCannotReplaceRetry() = scope.runTest {
        connect("https://nas.example/")
        val api = apis.last()
        api.nonCancellableDetail = true
        val model = DetailViewModel(7, sessions, dispatcher)
        runCurrent()
        val first = api.details.single()
        model.load()
        runCurrent()
        api.details.last().complete(AnimeDetail(7, "retry"))
        runCurrent()
        first.complete(AnimeDetail(7, "stale"))
        runCurrent()
        assertEquals("retry", model.state.value.title)
    }

    private fun TestScope.connect(url: String) {
        sessions.connect(ServerConnection.parse(url, false), "", "")
        runCurrent()
        apis.last().setup.complete(SetupState(false))
        runCurrent()
    }
}

private data class PageCall(val before: Long?, val day: Int?, val today: String?, val counts: Boolean)

private class GatedApi : AnimeApi(OkHttpClient(), ServerConnection.parse("https://nas.example/", false)) {
    val setup = CompletableDeferred<SetupState>()
    val pages = mutableListOf<CompletableDeferred<AnimePage>>()
    val pageCalls = mutableListOf<PageCall>()
    val details = mutableListOf<CompletableDeferred<AnimeDetail>>()
    var nonCancellableDetail = false
    override suspend fun setupStatus() = setup.await()
    override suspend fun probeSession() = Unit
    override suspend fun page(before: Long?, day: Int?, today: String?, counts: Boolean): AnimePage {
        pageCalls += PageCall(before, day, today, counts)
        return CompletableDeferred<AnimePage>().also { pages += it }.await()
    }
    override suspend fun detail(id: Long): AnimeDetail {
        val gate = CompletableDeferred<AnimeDetail>().also { details += it }
        return if (nonCancellableDetail) withContext(NonCancellable) { gate.await() } else gate.await()
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
