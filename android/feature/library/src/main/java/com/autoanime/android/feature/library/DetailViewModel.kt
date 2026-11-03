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
import com.autoanime.android.core.model.CorrectInput
import com.autoanime.android.core.model.allItems
import com.autoanime.android.core.network.AnimeApi
import com.autoanime.android.core.data.CallResult
import com.autoanime.android.core.data.call
import com.autoanime.android.core.data.connected
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.receiveAsFlow
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
    /** Every release of the anime; only saved ones are playable. */
    val episodes: List<MediaItem> = emptyList(),
    val bound: Boolean = false,
    val loaded: Boolean = false,
    val loading: Boolean = false,
    val error: AppError? = null,
    /** An item correction or rematch in flight. */
    val working: Boolean = false,
)

sealed interface DetailMessage {
    data class Corrected(val moved: Boolean, val warning: String) : DetailMessage
    data object RematchQueued : DetailMessage
    data class Failed(val error: AppError) : DetailMessage
}

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
    private val _messages = Channel<DetailMessage>(Channel.BUFFERED)
    val messages: Flow<DetailMessage> = _messages.receiveAsFlow()

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
                if (id == request) _state.value = detail.toUiState(session).copy(working = _state.value.working)
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

    /** Asks the server to look the anime up on Bangumi again; it runs in the background. */
    fun rematch() = act { rematch(animeId); DetailMessage.RematchQueued }

    /**
     * Moves [item] to another anime, season or episode. With [CorrectInput.rearchive] the server
     * also moves saved media, with its own rollback; otherwise only the record changes.
     */
    fun correct(item: MediaItem, input: CorrectInput) = act {
        val result = correctItem(item.id, input)
        DetailMessage.Corrected(result.moved, result.cleanup_warning)
    }

    private fun act(block: suspend AnimeApi.() -> DetailMessage) {
        val owner = sessions.connected ?: return
        if (_state.value.working) return
        _state.value = _state.value.copy(working = true)
        viewModelScope.launch {
            val result = sessions.call(owner, io, block)
            _state.value = _state.value.copy(working = false)
            when (result) {
                is CallResult.Success -> _messages.trySend(result.value)
                is CallResult.Failure -> _messages.trySend(DetailMessage.Failed(result.error))
                CallResult.Stale -> return@launch
            }
            load()
        }
    }
}

private fun AnimeDetail.toUiState(session: Session) = DetailUiState(
    title = title,
    summary = summary,
    coverUrl = cover.takeIf { it.isNotBlank() }?.let { session.resolve(it).toString() },
    episodes = allItems(),
    bound = bangumi_id > 0,
    loaded = true,
)
