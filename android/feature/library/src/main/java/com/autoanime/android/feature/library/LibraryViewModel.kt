package com.autoanime.android.feature.library

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.autoanime.android.core.data.IoDispatcher
import com.autoanime.android.core.data.Session
import com.autoanime.android.core.data.SessionRepository
import com.autoanime.android.core.data.SessionState
import com.autoanime.android.core.data.toAppError
import com.autoanime.android.core.model.Anime
import com.autoanime.android.core.model.AppError
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import javax.inject.Inject

data class AnimeCard(val id: Long, val title: String, val subtitle: String?, val season: Int, val coverUrl: String?)

data class LibraryUiState(
    val items: List<AnimeCard> = emptyList(),
    val loading: Boolean = false,
    /** A pull-to-refresh of an already populated list. */
    val refreshing: Boolean = false,
    val error: AppError? = null,
    val endReached: Boolean = false,
)

@HiltViewModel
class LibraryViewModel @Inject constructor(
    private val sessions: SessionRepository,
    @IoDispatcher private val io: CoroutineDispatcher,
) : ViewModel() {
    private val _state = MutableStateFlow(LibraryUiState())
    val state: StateFlow<LibraryUiState> = _state.asStateFlow()

    private var session: Session? = null
    private var cursor: Long? = null
    private var job: Job? = null

    init {
        viewModelScope.launch {
            sessions.state
                .map { (it as? SessionState.Connected)?.session }
                .distinctUntilChanged()
                .collect { next ->
                    job?.cancel()
                    session = next
                    cursor = null
                    _state.value = LibraryUiState()
                    if (next != null) load(next, before = null)
                }
        }
    }

    fun refresh() {
        session?.let { load(it, before = null) }
    }

    fun loadMore() {
        val current = session ?: return
        val before = cursor ?: return
        if (_state.value.loading) return
        load(current, before)
    }

    private fun load(owner: Session, before: Long?) {
        job?.cancel()
        _state.update { it.copy(loading = true, refreshing = before == null && it.items.isNotEmpty(), error = null) }
        job = viewModelScope.launch {
            try {
                val page = withContext(io) { owner.api.page(before) }
                // A page requested by an earlier session must not reach the new one.
                if (session !== owner) return@launch
                cursor = page.next_before.takeIf { it > 0 }
                _state.update { current ->
                    val previous = if (before == null) emptyList() else current.items
                    val seen = previous.mapTo(HashSet()) { it.id }
                    current.copy(
                        items = previous + page.items.filter { seen.add(it.id) }.map { it.toCard(owner) },
                        loading = false,
                        refreshing = false,
                        endReached = cursor == null,
                    )
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                if (session !== owner) return@launch
                val error = e.toAppError()
                if (error == AppError.SessionExpired) sessions.onUnauthorized(owner)
                else _state.update { it.copy(loading = false, refreshing = false, error = error) }
            }
        }
    }
}

internal fun Anime.toCard(session: Session) = AnimeCard(
    id = id,
    title = title,
    subtitle = meta_name_cn.ifBlank { meta_name }.ifBlank { null }?.takeIf { it != title },
    season = season,
    coverUrl = cover.takeIf { it.isNotBlank() }?.let { session.resolve(it).toString() },
)
