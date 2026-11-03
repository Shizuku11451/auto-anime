package com.autoanime.android.feature.library

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.autoanime.android.core.data.IoDispatcher
import com.autoanime.android.core.data.Session
import com.autoanime.android.core.data.SessionRepository
import com.autoanime.android.core.data.SessionState
import com.autoanime.android.core.data.toAppError
import com.autoanime.android.core.model.AnimeDetail
import com.autoanime.android.core.model.AppError
import com.autoanime.android.core.model.MediaItem
import com.autoanime.android.core.model.savedItems
import dagger.assisted.Assisted
import dagger.assisted.AssistedFactory
import dagger.assisted.AssistedInject
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

data class DetailUiState(
    val title: String = "",
    val summary: String = "",
    val coverUrl: String? = null,
    val episodes: List<MediaItem> = emptyList(),
    val loaded: Boolean = false,
    val loading: Boolean = false,
    val error: AppError? = null,
)

@HiltViewModel(assistedFactory = DetailViewModel.Factory::class)
class DetailViewModel @AssistedInject constructor(
    @Assisted val animeId: Long,
    private val sessions: SessionRepository,
    @IoDispatcher private val io: CoroutineDispatcher,
) : ViewModel() {
    @AssistedFactory
    interface Factory {
        fun create(animeId: Long): DetailViewModel
    }

    private val _state = MutableStateFlow(DetailUiState(loading = true))
    val state: StateFlow<DetailUiState> = _state.asStateFlow()
    private var job: Job? = null
    private var request = 0L

    init {
        load()
    }

    fun load() {
        val session = (sessions.state.value as? SessionState.Connected)?.session ?: return
        job?.cancel()
        val id = ++request
        _state.value = _state.value.copy(loading = true, error = null)
        job = viewModelScope.launch {
            try {
                val detail = withContext(io) { session.api.detail(animeId) }
                if (id == request) _state.value = detail.toUiState(session)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                if (id != request) return@launch
                val error = e.toAppError()
                if (error == AppError.SessionExpired) sessions.onUnauthorized(session)
                else _state.value = _state.value.copy(loading = false, error = error)
            }
        }
    }
}

private fun AnimeDetail.toUiState(session: Session) = DetailUiState(
    title = title,
    summary = summary,
    coverUrl = cover.takeIf { it.isNotBlank() }?.let { session.resolve(it).toString() },
    episodes = savedItems(),
    loaded = true,
)
