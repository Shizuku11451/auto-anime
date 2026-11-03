package com.autoanime.android.feature.downloads

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.autoanime.android.core.data.IoDispatcher
import com.autoanime.android.core.data.Session
import com.autoanime.android.core.data.SessionRepository
import com.autoanime.android.core.data.SessionState
import com.autoanime.android.core.data.toAppError
import com.autoanime.android.core.model.AppError
import com.autoanime.android.core.model.DownloadAction
import com.autoanime.android.core.model.DownloadItem
import com.autoanime.android.core.model.StatusGroup
import com.autoanime.android.core.model.hasActiveDownloads
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Job
import kotlinx.coroutines.async
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import javax.inject.Inject
import kotlin.random.Random

data class DownloadRow(val item: DownloadItem, val coverUrl: String?)

data class DownloadsUiState(
    val group: StatusGroup = StatusGroup.All,
    val query: String = "",
    val items: List<DownloadRow> = emptyList(),
    /** Library-wide counts by raw status; null until the first successful load. */
    val counts: Map<String, Int>? = null,
    val loading: Boolean = false,
    /** A user-requested refresh of an already populated list. */
    val refreshing: Boolean = false,
    /** The list could not be loaded at all. */
    val error: AppError? = null,
    /** Loading the next page failed; the loaded rows stay. */
    val appendError: AppError? = null,
    /** The latest background update failed, so the rows may be out of date. */
    val stale: Boolean = false,
    val endReached: Boolean = false,
    /** Items with an action in flight; their controls are disabled. */
    val busy: Set<Long> = emptySet(),
)

sealed interface DownloadMessage {
    data class Done(val action: DownloadAction) : DownloadMessage
    /** The server already had the task in the requested state. */
    data object Unchanged : DownloadMessage
    data object Removed : DownloadMessage
    data class Failed(val error: AppError) : DownloadMessage
}

/**
 * Lists releases with their download state and forwards task actions to the server, which stays
 * the only authority on task identity and transitions. While the screen is visible and the server
 * reports active or queued work, the loaded range is re-fetched about every two seconds; failures
 * back off up to a minute. Requests never overlap, and every result is checked against the session
 * and the filter it was made for, so a late response cannot replace a newer list.
 */
@HiltViewModel
class DownloadsViewModel @Inject constructor(
    private val sessions: SessionRepository,
    @IoDispatcher private val io: CoroutineDispatcher,
) : ViewModel() {
    private val _state = MutableStateFlow(DownloadsUiState())
    val state: StateFlow<DownloadsUiState> = _state.asStateFlow()

    private val _messages = Channel<DownloadMessage>(Channel.BUFFERED)
    val messages: Flow<DownloadMessage> = _messages.receiveAsFlow()

    private var session: Session? = null
    /** Advances whenever the session, filter or search changes; older results are dropped. */
    private var listVersion = 0
    /** Advances with every list request so only the newest one publishes. */
    private var loadToken = 0
    private var cursor: Long? = null
    private var listJob: Job? = null
    private var queryJob: Job? = null
    private var pollJob: Job? = null
    private var visible = false

    init {
        viewModelScope.launch {
            sessions.state
                .map { (it as? SessionState.Connected)?.session }
                .distinctUntilChanged()
                .collect { next ->
                    stopPolling()
                    listJob?.cancel()
                    queryJob?.cancel()
                    session = next
                    listVersion++
                    cursor = null
                    _state.value = DownloadsUiState()
                    if (next != null) reload()
                }
        }
    }

    /** Polling runs only while the screen is started. */
    fun setVisible(value: Boolean) {
        if (visible == value) return
        visible = value
        if (!value) stopPolling()
        else if (session != null && listJob?.isActive != true) refresh(userInitiated = false)
    }

    fun selectGroup(group: StatusGroup) {
        if (_state.value.group == group) return
        queryJob?.cancel()
        _state.update { it.copy(group = group) }
        reload()
    }

    fun setQuery(text: String) {
        if (_state.value.query == text) return
        _state.update { it.copy(query = text) }
        queryJob?.cancel()
        queryJob = viewModelScope.launch {
            delay(SEARCH_DEBOUNCE_MS)
            reload()
        }
    }

    fun refresh() = refresh(userInitiated = true)

    fun loadMore() {
        val owner = session ?: return
        val before = cursor ?: return
        if (listJob?.isActive == true) return
        val version = listVersion
        val token = ++loadToken
        val current = _state.value
        _state.update { it.copy(appendError = null) }
        listJob = viewModelScope.launch {
            try {
                val page = withContext(io) { owner.api.items(current.group.query, current.query, before) }
                if (!publishes(owner, version, token)) return@launch
                cursor = page.next_before.takeIf { it > 0 }
                _state.update { state ->
                    val seen = state.items.mapTo(HashSet()) { it.item.id }
                    state.copy(
                        items = state.items + page.items.filter { seen.add(it.id) }.map { it.toRow(owner) },
                        endReached = cursor == null,
                    )
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                if (!publishes(owner, version, token)) return@launch
                val error = e.toAppError()
                if (error == AppError.SessionExpired) sessions.onUnauthorized(owner)
                else _state.update { it.copy(appendError = error) }
            }
        }
    }

    fun perform(row: DownloadRow, action: DownloadAction) = runAction(row) { owner ->
        val changed = owner.api.itemAction(row.item.id, action)
        if (changed) DownloadMessage.Done(action) else DownloadMessage.Unchanged
    }

    /** Removes only the server record; saved files and downloader sources are kept. */
    fun remove(row: DownloadRow) = runAction(row) { owner ->
        owner.api.deleteItem(row.item.id)
        DownloadMessage.Removed
    }

    private fun runAction(row: DownloadRow, call: suspend (Session) -> DownloadMessage) {
        val owner = session ?: return
        val id = row.item.id
        if (id in _state.value.busy) return
        _state.update { it.copy(busy = it.busy + id) }
        viewModelScope.launch {
            val message = try {
                withContext(io) { call(owner) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                DownloadMessage.Failed(e.toAppError())
            }
            if (session !== owner) return@launch
            _state.update { it.copy(busy = it.busy - id) }
            if (message is DownloadMessage.Failed && message.error == AppError.SessionExpired) {
                sessions.onUnauthorized(owner)
                return@launch
            }
            _messages.trySend(message)
            // Submitting is not completion: show what the server actually recorded.
            refresh(userInitiated = false)
        }
    }

    /** Starts over for the current filter and search. */
    private fun reload() {
        listJob?.cancel()
        listVersion++
        cursor = null
        _state.update {
            it.copy(items = emptyList(), error = null, appendError = null, stale = false, endReached = false, refreshing = false)
        }
        refresh(userInitiated = false)
    }

    /**
     * Re-fetches from the top through the oldest loaded row (bounded by [MAX_REFRESH_PAGES]) and
     * keeps any rows beyond that range, so polling does not collapse a long scrolled list.
     */
    private fun refresh(userInitiated: Boolean): Job? {
        val owner = session ?: return null
        listJob?.cancel()
        val version = listVersion
        val token = ++loadToken
        val current = _state.value
        val oldest = current.items.lastOrNull()?.item?.id
        _state.update {
            it.copy(loading = it.items.isEmpty(), refreshing = userInitiated && it.items.isNotEmpty(), error = null)
        }
        return viewModelScope.launch {
            try {
                val (status, fetched, next) = withContext(io) {
                    coroutineScope {
                        val status = async { owner.api.runtimeStatus() }
                        val fetched = ArrayList<DownloadItem>()
                        var before: Long? = null
                        var pages = 0
                        do {
                            val page = owner.api.items(current.group.query, current.query, before)
                            fetched += page.items
                            before = page.next_before.takeIf { it > 0 }
                            pages++
                        } while (before != null && oldest != null && before > oldest && pages < MAX_REFRESH_PAGES)
                        Triple(status.await(), fetched, before)
                    }
                }
                if (!publishes(owner, version, token)) return@launch
                val seen = HashSet<Long>()
                val head = fetched.filter { seen.add(it.id) }.map { it.toRow(owner) }
                // Rows below the refreshed range were not re-checked but are still in order.
                val tail = if (next == null) emptyList()
                else _state.value.items.filter { it.item.id < next && seen.add(it.item.id) }
                cursor = if (tail.isEmpty()) next else cursor
                _state.update {
                    it.copy(
                        items = head + tail,
                        counts = status.counts,
                        loading = false,
                        refreshing = false,
                        error = null,
                        stale = false,
                        endReached = cursor == null,
                    )
                }
                if (hasActiveDownloads(status.counts)) startPolling()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                if (!publishes(owner, version, token)) return@launch
                val error = e.toAppError()
                if (error == AppError.SessionExpired) {
                    sessions.onUnauthorized(owner)
                    return@launch
                }
                _state.update {
                    if (it.items.isEmpty()) it.copy(loading = false, refreshing = false, error = error)
                    else it.copy(loading = false, refreshing = false, stale = true)
                }
                if (userInitiated && _state.value.items.isNotEmpty()) _messages.trySend(DownloadMessage.Failed(error))
                startPolling()
            }
        }.also { listJob = it }
    }

    private fun startPolling() {
        if (!visible || pollJob?.isActive == true) return
        pollJob = viewModelScope.launch {
            var failures = 0
            while (true) {
                delay(if (failures == 0) POLL_INTERVAL_MS else backoff(failures))
                // A request already in flight (user refresh, action, paging) counts as this tick.
                (listJob?.takeIf { it.isActive } ?: refresh(userInitiated = false))?.join()
                val state = _state.value
                when {
                    state.stale || state.error != null -> failures++
                    state.counts?.let(::hasActiveDownloads) == true -> failures = 0
                    else -> break
                }
            }
        }
    }

    private fun stopPolling() {
        pollJob?.cancel()
        pollJob = null
    }

    private fun publishes(owner: Session, version: Int, token: Int) =
        session === owner && listVersion == version && loadToken == token

    private fun backoff(failures: Int): Long {
        val base = (POLL_INTERVAL_MS shl failures.coerceAtMost(5)).coerceAtMost(MAX_BACKOFF_MS)
        return base / 2 + Random.nextLong(base / 2 + 1)
    }

    override fun onCleared() {
        stopPolling()
        queryJob?.cancel()
    }

    companion object {
        const val POLL_INTERVAL_MS = 2_000L
        const val MAX_BACKOFF_MS = 60_000L
        const val SEARCH_DEBOUNCE_MS = 300L
        /** Polling re-checks at most this many pages; deeper rows refresh on scroll or pull. */
        const val MAX_REFRESH_PAGES = 3
    }
}

internal fun DownloadItem.toRow(session: Session) = DownloadRow(
    item = this,
    coverUrl = cover.takeIf { it.isNotBlank() }?.let { session.resolve(it).toString() },
)
