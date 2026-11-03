package com.autoanime.android.feature.discover

import com.autoanime.android.core.model.AggregateResource
import com.autoanime.android.core.model.AggregateResult
import com.autoanime.android.core.model.Feed
import com.autoanime.android.core.model.FeedInput
import com.autoanime.android.core.model.FeedSaveResult
import com.autoanime.android.core.model.SEASON_UNMARKED
import com.autoanime.android.core.model.SyncResult
import com.autoanime.android.core.model.aggregateUrl
import com.autoanime.android.core.model.subscriptionRules
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
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class SubjectViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val scope = TestScope(dispatcher)
    private val api = FakeDiscoverApi()

    @Before fun setUp() = Dispatchers.setMain(dispatcher)
    @After fun tearDown() = Dispatchers.resetMain()

    @Test fun releasesGroupIntoVersionsAndExistingSubscriptionsAreMarked() = scope.runTest {
        val subscribedRules = requireNotNull(release("01", group = "B").subscriptionRules(7))
        api.feeds = listOf(Feed(9, "mine", url = aggregateUrl(7, subscribedRules)))
        api.resources = AggregateResult(
            7,
            listOf(release("01"), release("02"), release("01", group = "B"), release("03", magnet = "bad")),
            warnings = listOf("source x failed"),
        )
        val model = SubjectViewModel(7, "芙莉莲", connectedSessions(dispatcher, api), dispatcher)
        runCurrent()
        val state = model.state.value
        assertEquals(listOf("A" to 2, "B" to 1), state.versions.map { it.group to it.releases.size })
        assertNull(state.versions[0].subscribedFeedId)
        assertEquals(9L, state.versions[1].subscribedFeedId)
        assertEquals(1, state.unusable)
        assertEquals(listOf("source x failed"), state.warnings)
        assertEquals("1–2", episodeRange(state.versions[0]))
    }

    @Test fun subscribingUsesStrictSeasonZeroOffsetAndSyncsOnce() = scope.runTest {
        api.resources = AggregateResult(7, listOf(release("01", season = 0)))
        val model = SubjectViewModel(7, "芙莉莲", connectedSessions(dispatcher, api), dispatcher)
        runCurrent()
        val version = model.state.value.versions.single()
        model.subscribe(version, " ", autoDownload = false)
        model.subscribe(version, " ", autoDownload = false)
        runCurrent()
        val input = api.created.single()
        assertEquals("芙莉莲 · A · 1080p", input.name)
        assertEquals("芙莉莲", input.target_title)
        assertEquals(SEASON_UNMARKED, input.season_filter)
        assertEquals(0, input.episode_offset)
        assertEquals(false, input.auto_download)
        assertEquals(aggregateUrl(7, version.rules), input.url)
        assertEquals(listOf(41L), api.synced)
    }

    private fun release(episode: String, group: String = "A", season: Int = 1, magnet: String = "magnet:?xt=urn:btih:" + "b".repeat(40)) =
        AggregateResource(
            title = "[$group] 芙莉莲 - $episode [1080p]",
            anime_title = "芙莉莲",
            fansub_group = group,
            episode = episode,
            quality = "1080p",
            season = season,
            published = "2026-10-0$episode",
            magnet = magnet,
        )
}

private class FakeDiscoverApi : BaseFakeApi() {
    var feeds: List<Feed> = emptyList()
    var resources = AggregateResult()
    val created = mutableListOf<FeedInput>()
    val synced = mutableListOf<Long>()
    override suspend fun feeds() = feeds
    override suspend fun discoverResources(subjectId: Long) = resources
    override suspend fun createFeed(input: FeedInput): FeedSaveResult {
        created += input
        return FeedSaveResult(id = 41)
    }
    override suspend fun syncFeed(id: Long): SyncResult {
        synced += id
        return SyncResult(1)
    }
}
