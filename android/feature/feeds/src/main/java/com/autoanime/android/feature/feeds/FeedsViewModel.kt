package com.autoanime.android.feature.feeds

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.autoanime.android.core.data.CallResult
import com.autoanime.android.core.data.IoDispatcher
import com.autoanime.android.core.data.SessionRepository
import com.autoanime.android.core.data.call
import com.autoanime.android.core.data.connected
import com.autoanime.android.core.model.AppError
import com.autoanime.android.core.model.Feed
import com.autoanime.android.core.model.FeedInput
import com.autoanime.android.core.network.AnimeApi
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Job
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class FeedsUiState(
    val feeds: List<Feed> = emptyList(),
    val loaded: Boolean = false,
    val loading: Boolean = false,
    val error: AppError? = null,
    /** Subscriptions with an action in flight. */
    val busy: Set<Long> = emptySet(),
)

sealed interface FeedMessage {
    data class Synced(val added: Int) : FeedMessage
    data class Toggled(val enabled: Boolean) : FeedMessage
    data object Deleted : FeedMessage
    data class Failed(val error: AppError) : FeedMessage
}

/** Subscription list with enable/disable, manual sync and deletion; every action re-reads the list. */
@HiltViewModel
class FeedsViewModel @Inject constructor(
    private val sessions: SessionRepository,
    @IoDispatcher private val io: CoroutineDispatcher,
) : ViewModel() {
    private val _state = MutableStateFlow(FeedsUiState())
    val state: StateFlow<FeedsUiState> = _state.asStateFlow()
    private val _messages = Channel<FeedMessage>(Channel.BUFFERED)
    val messages: Flow<FeedMessage> = _messages.receiveAsFlow()
    private var loadJob: Job? = null

    fun load() {
        val owner = sessions.connected ?: return
        loadJob?.cancel()
        _state.update { it.copy(loading = true, error = null) }
        loadJob = viewModelScope.launch {
            when (val result = sessions.call(owner, io) { feeds() }) {
                is CallResult.Success -> _state.update { it.copy(feeds = result.value, loaded = true, loading = false) }
                is CallResult.Failure -> _state.update { it.copy(loading = false, error = result.error) }
                CallResult.Stale -> Unit
            }
        }
    }

    fun toggle(feed: Feed) = act(feed) {
        // The server requires these four fields on every update; the rest keep their stored values.
        updateFeed(feed.id, FeedInput(feed.name, feed.include_words, feed.exclude_words, feed.auto_download, enabled = !feed.enabled))
        FeedMessage.Toggled(!feed.enabled)
    }

    fun sync(feed: Feed) = act(feed) { FeedMessage.Synced(syncFeed(feed.id).added) }

    /** Removes the subscription and its item records; the server keeps saved media. */
    fun delete(feed: Feed) = act(feed) {
        deleteFeed(feed.id)
        FeedMessage.Deleted
    }

    private fun act(feed: Feed, block: suspend AnimeApi.() -> FeedMessage) {
        val owner = sessions.connected ?: return
        if (feed.id in _state.value.busy) return
        _state.update { it.copy(busy = it.busy + feed.id) }
        viewModelScope.launch {
            val result = sessions.call(owner, io, block)
            _state.update { it.copy(busy = it.busy - feed.id) }
            when (result) {
                is CallResult.Success -> _messages.trySend(result.value)
                is CallResult.Failure -> _messages.trySend(FeedMessage.Failed(result.error))
                CallResult.Stale -> return@launch
            }
            load()
        }
    }
}
