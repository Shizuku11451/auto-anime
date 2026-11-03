package com.autoanime.android.feature.feeds

import com.autoanime.android.core.model.Feed
import com.autoanime.android.core.model.FeedInput
import com.autoanime.android.core.model.FeedSaveResult
import com.autoanime.android.core.model.PreviewInput
import com.autoanime.android.core.model.PreviewResult
import com.autoanime.android.core.model.SEASON_UNMARKED
import com.autoanime.android.core.model.SyncResult
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class FeedEditorViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val scope = TestScope(dispatcher)
    private val api = FakeFeedsApi()

    @Before fun setUp() = Dispatchers.setMain(dispatcher)
    @After fun tearDown() = Dispatchers.resetMain()

    @Test fun changingRulesDropsThePreviewAndItsLateResult() = scope.runTest {
        val model = FeedEditorViewModel(null, connectedSessions(dispatcher, api), dispatcher)
        model.update { it.copy(name = "x", url = "https://rss.example/feed") }
        model.preview()
        runCurrent()
        model.update { it.copy(include = "1080p") }
        api.previews.single().complete(PreviewResult(total = 9, count = 9))
        runCurrent()
        assertNull(model.state.value.preview)
        assertTrue(!model.state.value.previewing)
    }

    @Test fun renamingKeepsThePreview() = scope.runTest {
        val model = FeedEditorViewModel(null, connectedSessions(dispatcher, api), dispatcher)
        model.update { it.copy(url = "https://rss.example/feed") }
        model.preview()
        runCurrent()
        api.previews.single().complete(PreviewResult(total = 3, count = 2))
        runCurrent()
        model.update { it.copy(name = "renamed", autoDownload = false) }
        assertEquals(2, model.state.value.preview?.count)
    }

    @Test fun newSubscriptionNeedsAnHttpAddressAndSyncsOnce() = scope.runTest {
        val model = FeedEditorViewModel(null, connectedSessions(dispatcher, api), dispatcher)
        model.update { it.copy(name = "x", url = "ftp://rss.example/feed") }
        model.save()
        runCurrent()
        assertEquals(FormProblem.Url, model.state.value.problem)
        assertTrue(api.created.isEmpty())

        model.update { it.copy(url = " https://rss.example/feed ", seasonMode = SeasonMode.Unmarked) }
        model.save()
        runCurrent()
        val input = api.created.single()
        assertEquals("https://rss.example/feed", input.url)
        assertEquals(SEASON_UNMARKED, input.season_filter)
        assertEquals(listOf(41L), api.synced)
        assertEquals(SaveOutcome(created = true, added = 3, storageWarning = ""), model.state.value.saved)
    }

    @Test fun failedFirstSyncStillCountsAsSaved() = scope.runTest {
        api.syncFails = true
        val model = FeedEditorViewModel(null, connectedSessions(dispatcher, api), dispatcher)
        model.update { it.copy(name = "x", url = "https://rss.example/feed") }
        model.save()
        runCurrent()
        assertEquals(SaveOutcome(created = true, added = null, storageWarning = ""), model.state.value.saved)
    }

    @Test fun editingKeepsTheAddressAndUnsentFieldsOnTheServer() = scope.runTest {
        api.feeds = listOf(Feed(5, "old", url = "https://rss.example/private?token=t", season_filter = 2, allow_unknown_season = true, enabled = false))
        val model = FeedEditorViewModel(5, connectedSessions(dispatcher, api), dispatcher)
        runCurrent()
        assertEquals(SeasonMode.Specific, model.state.value.form.seasonMode)
        assertEquals("2", model.state.value.form.season)
        model.update { it.copy(name = "new") }
        model.save()
        runCurrent()
        val (id, input) = api.updated.single()
        assertEquals(5L, id)
        assertNull("the address cannot change", input.url)
        assertNull("the enable switch lives in the list", input.enabled)
        assertEquals(2, input.season_filter)
        assertNotNull(model.state.value.saved)
        assertTrue(api.synced.isEmpty())
    }

    @Test fun seasonOutOfRangeIsRejectedBeforeSaving() = scope.runTest {
        val model = FeedEditorViewModel(null, connectedSessions(dispatcher, api), dispatcher)
        model.update { it.copy(name = "x", url = "https://rss.example/feed", seasonMode = SeasonMode.Specific, season = "0") }
        model.save()
        runCurrent()
        assertEquals(FormProblem.Season, model.state.value.problem)
        assertTrue(api.created.isEmpty())
    }
}

private class FakeFeedsApi : BaseFakeApi() {
    var feeds: List<Feed> = emptyList()
    val previews = mutableListOf<CompletableDeferred<PreviewResult>>()
    val created = mutableListOf<FeedInput>()
    val updated = mutableListOf<Pair<Long, FeedInput>>()
    val synced = mutableListOf<Long>()
    var syncFails = false
    override suspend fun feeds() = feeds
    override suspend fun preview(input: PreviewInput): PreviewResult = CompletableDeferred<PreviewResult>().also { previews += it }.await()
    override suspend fun createFeed(input: FeedInput): FeedSaveResult {
        created += input
        return FeedSaveResult(id = 41)
    }
    override suspend fun updateFeed(id: Long, input: FeedInput): FeedSaveResult {
        updated += id to input
        return FeedSaveResult(id = id)
    }
    override suspend fun syncFeed(id: Long): SyncResult {
        synced += id
        if (syncFails) throw java.io.IOException("offline")
        return SyncResult(3)
    }
}
