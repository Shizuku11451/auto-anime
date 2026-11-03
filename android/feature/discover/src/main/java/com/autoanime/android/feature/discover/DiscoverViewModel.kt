package com.autoanime.android.feature.discover

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.autoanime.android.core.data.CallResult
import com.autoanime.android.core.data.IoDispatcher
import com.autoanime.android.core.data.SessionRepository
import com.autoanime.android.core.data.call
import com.autoanime.android.core.data.connected
import com.autoanime.android.core.model.AppError
import com.autoanime.android.core.model.DiscoverSubject
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class DiscoverUiState(
    val query: String = "",
    /** The keyword of the results on screen, which may differ from what is being typed. */
    val searched: String = "",
    val searching: Boolean = false,
    val error: AppError? = null,
    val subjects: List<SubjectCard> = emptyList(),
    /** False when the server has no aggregate search service; subjects can still be browsed. */
    val resourcesEnabled: Boolean = true,
)

data class SubjectCard(val subject: DiscoverSubject, val coverUrl: String?)

/** Bangumi subject search. A newer search replaces an older one, whose late result is dropped. */
@HiltViewModel
class DiscoverViewModel @Inject constructor(
    private val sessions: SessionRepository,
    @IoDispatcher private val io: CoroutineDispatcher,
) : ViewModel() {
    private val _state = MutableStateFlow(DiscoverUiState())
    val state: StateFlow<DiscoverUiState> = _state.asStateFlow()
    private var job: Job? = null
    private var token = 0

    fun setQuery(text: String) = _state.update { it.copy(query = text) }

    fun search() {
        val owner = sessions.connected ?: return
        val keyword = _state.value.query.trim()
        if (keyword.isEmpty()) return
        job?.cancel()
        val current = ++token
        _state.update { it.copy(searching = true, error = null) }
        job = viewModelScope.launch {
            val result = sessions.call(owner, io) { discoverSubjects(keyword) }
            if (current != token) return@launch
            when (result) {
                is CallResult.Success -> _state.update {
                    it.copy(
                        searching = false,
                        searched = keyword,
                        subjects = result.value.items.map { s ->
                            SubjectCard(s, s.cover.takeIf(String::isNotBlank)?.let(owner::resolveCover))
                        },
                        resourcesEnabled = result.value.resources_enabled,
                    )
                }
                is CallResult.Failure -> _state.update { it.copy(searching = false, error = result.error) }
                CallResult.Stale -> Unit
            }
        }
    }
}

/**
 * Server covers are relative paths loaded with the session. Search results usually link Bangumi's
 * CDN instead; the session's loader only reaches the selected server, so those show a placeholder.
 */
internal fun com.autoanime.android.core.data.Session.resolveCover(url: String): String? =
    if (url.startsWith("http://") || url.startsWith("https://")) null else resolve(url).toString()
