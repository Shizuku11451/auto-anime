package com.autoanime.android.feature.discover

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.autoanime.android.core.data.CallResult
import com.autoanime.android.core.data.IoDispatcher
import com.autoanime.android.core.data.SessionRepository
import com.autoanime.android.core.data.call
import com.autoanime.android.core.data.connected
import com.autoanime.android.core.model.AggregateResource
import com.autoanime.android.core.model.AppError
import com.autoanime.android.core.model.FeedInput
import com.autoanime.android.core.model.SEASON_UNMARKED
import com.autoanime.android.core.model.aggregateUrl
import com.autoanime.android.core.model.findAggregate
import com.autoanime.android.core.model.subscriptionRules
import dagger.assisted.Assisted
import dagger.assisted.AssistedFactory
import dagger.assisted.AssistedInject
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.async
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * Releases that one aggregate subscription would follow: same title, group, quality, language
 * and season. [subscribedFeedId] is set when a subscription for exactly these rules exists.
 */
data class ResourceVersion(
    val rules: Map<String, String>,
    val group: String,
    val quality: String,
    val language: String,
    val season: Int,
    val releases: List<AggregateResource>,
    val subscribedFeedId: Long?,
) {
    val key: String get() = rules.toString()
    val needsReview: Boolean get() = releases.any { it.needs_review }
}

data class SubjectUiState(
    val loading: Boolean = true,
    val error: AppError? = null,
    val versions: List<ResourceVersion> = emptyList(),
    /** Releases the aggregate service returned that cannot back a subscription. */
    val unusable: Int = 0,
    /** Sources that failed; the remaining results are still shown. */
    val warnings: List<String> = emptyList(),
    val subscribing: Set<String> = emptySet(),
)

sealed interface SubjectMessage {
    data class Subscribed(val added: Int?) : SubjectMessage
    data class Failed(val error: AppError) : SubjectMessage
}

/** Aggregate resources for one Bangumi subject, and subscribing to one version of them. */
@HiltViewModel(assistedFactory = SubjectViewModel.Factory::class)
class SubjectViewModel @AssistedInject constructor(
    @Assisted private val subjectId: Long,
    @Assisted private val subjectName: String,
    private val sessions: SessionRepository,
    @IoDispatcher private val io: CoroutineDispatcher,
) : ViewModel() {
    @AssistedFactory
    interface Factory {
        fun create(subjectId: Long, subjectName: String): SubjectViewModel
    }

    private val _state = MutableStateFlow(SubjectUiState())
    val state: StateFlow<SubjectUiState> = _state.asStateFlow()
    private val _messages = Channel<SubjectMessage>(Channel.BUFFERED)
    val messages: Flow<SubjectMessage> = _messages.receiveAsFlow()

    init {
        load()
    }

    fun load() {
        val owner = sessions.connected ?: return
        _state.update { it.copy(loading = true, error = null) }
        viewModelScope.launch {
            val result = sessions.call(owner, io) {
                coroutineScope {
                    val feeds = async { feeds() }
                    val resources = discoverResources(subjectId)
                    resources to feeds.await()
                }
            }
            when (result) {
                is CallResult.Success -> {
                    val (resources, feeds) = result.value
                    val usable = resources.items.mapNotNull { r -> r.subscriptionRules(subjectId)?.let { it to r } }
                    val versions = usable.groupBy({ it.first }, { it.second }).map { (rules, releases) ->
                        ResourceVersion(
                            rules = rules,
                            group = rules["group"].orEmpty(),
                            quality = rules["quality"].orEmpty(),
                            language = rules["language"].orEmpty(),
                            season = rules["season"]?.toIntOrNull() ?: 0,
                            releases = releases,
                            subscribedFeedId = feeds.findAggregate(subjectId, rules)?.id,
                        )
                    }.sortedByDescending { it.releases.size }
                    _state.update {
                        it.copy(
                            loading = false,
                            versions = versions,
                            unusable = resources.items.size - usable.size,
                            warnings = resources.warnings,
                        )
                    }
                }
                is CallResult.Failure -> _state.update { it.copy(loading = false, error = result.error) }
                CallResult.Stale -> Unit
            }
        }
    }

    /**
     * Creates the subscription the same way the web client does: episode offset 0 (search results
     * are not a complete numbering baseline) and a strict season rule, then syncs it once.
     */
    fun subscribe(version: ResourceVersion, name: String, autoDownload: Boolean) {
        val owner = sessions.connected ?: return
        if (version.key in _state.value.subscribing || version.subscribedFeedId != null) return
        _state.update { it.copy(subscribing = it.subscribing + version.key) }
        val input = FeedInput(
            name = name.trim().ifEmpty { defaultName(version) },
            include_words = "",
            exclude_words = "",
            auto_download = autoDownload,
            url = aggregateUrl(subjectId, version.rules),
            target_title = subjectName,
            season_filter = if (version.season > 0) version.season else SEASON_UNMARKED,
            episode_offset = 0,
        )
        viewModelScope.launch {
            val result = sessions.call(owner, io) {
                val created = createFeed(input)
                // The subscription exists even if this first sync fails; it syncs again on schedule.
                try {
                    syncFeed(created.id).added
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    null
                }
            }
            _state.update { it.copy(subscribing = it.subscribing - version.key) }
            when (result) {
                is CallResult.Success -> {
                    _messages.trySend(SubjectMessage.Subscribed(result.value))
                    load()
                }
                is CallResult.Failure -> _messages.trySend(SubjectMessage.Failed(result.error))
                CallResult.Stale -> Unit
            }
        }
    }

    fun defaultName(version: ResourceVersion): String =
        listOf(subjectName, version.group, version.quality, version.language).filter(String::isNotBlank).joinToString(" · ")
}
