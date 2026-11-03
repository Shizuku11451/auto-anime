package com.autoanime.android.feature.player

import android.content.Context
import androidx.annotation.OptIn
import androidx.annotation.StringRes
import androidx.annotation.VisibleForTesting
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.media3.common.AudioAttributes
import androidx.media3.common.C
import androidx.media3.common.MediaItem
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.HttpDataSource
import androidx.media3.datasource.okhttp.OkHttpDataSource
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.source.ProgressiveMediaSource
import com.autoanime.android.core.data.AppPreferences
import com.autoanime.android.core.data.Session
import com.autoanime.android.core.data.SessionRepository
import com.autoanime.android.core.data.SessionState
import com.autoanime.android.core.database.WatchProgressPolicy
import com.autoanime.android.core.database.WatchProgressRepository
import com.autoanime.android.core.model.PlayableEpisode
import dagger.assisted.Assisted
import dagger.assisted.AssistedFactory
import dagger.assisted.AssistedInject
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import java.util.concurrent.atomic.AtomicInteger

data class AutoNextState(val label: String, val secondsLeft: Int)

data class PlayerUiState(
    @StringRes val error: Int? = null,
    val firstFrameRendered: Boolean = false,
    val lastSeekPositionMs: Long? = null,
    val buffering: Boolean = false,
    /** The position playback resumed from; shown once as a hint and then cleared. */
    val resumeHintMs: Long? = null,
    val autoNext: AutoNextState? = null,
    val currentLabel: String = "",
    val hasNext: Boolean = false,
    val hasPrev: Boolean = false,
    val playbackRate: Float = 1f,
)

/**
 * Plays a queue of saved releases. The ExoPlayer only lives while the screen is started; the
 * playback position is kept per item in the on-device watch-progress store and restored on reopen,
 * and the player instance survives switching between queue entries.
 */
@HiltViewModel(assistedFactory = PlayerViewModel.Factory::class)
class PlayerViewModel @AssistedInject constructor(
    @Assisted episodes: List<PlayableEpisode>,
    @Assisted startIndex: Int,
    private val sessions: SessionRepository,
    private val watchProgress: WatchProgressRepository,
    private val preferences: AppPreferences,
) : ViewModel() {
    @AssistedFactory
    interface Factory {
        fun create(episodes: List<PlayableEpisode>, startIndex: Int): PlayerViewModel
    }

    private val queue = PlayerQueue(
        episodes.map { PlayerQueue.Entry(it.id, it.label, it.animeId) },
        startIndex,
    )

    private val _state = MutableStateFlow(
        PlayerUiState(
            currentLabel = queue.current.label,
            hasNext = queue.hasNext,
            hasPrev = queue.hasPrev,
        ),
    )
    val state: StateFlow<PlayerUiState> = _state.asStateFlow()

    /** The live player, created on screen start and released on stop; null while building. */
    private val _player = MutableStateFlow<ExoPlayer?>(null)
    val player: StateFlow<ExoPlayer?> = _player.asStateFlow()

    private val session: Session? = (sessions.state.value as? SessionState.Connected)?.session
    private var creationJob: Job? = null
    private var tickerJob: Job? = null
    private var autoNextJob: Job? = null
    private var playWhenReady = true

    /** The position the current media source was started at, for the resume hint and file-change guard. */
    private var startMs = 0L

    fun createPlayer(context: Context) {
        if (_player.value != null || creationJob?.isActive == true) return
        creationJob = viewModelScope.launch {
            val built = buildPlayer(context) ?: return@launch
            // No suspension between building the player and publishing it, so a release() racing
            // on stop either sees the previous null state or the finished player.
            _player.value = built
        }
    }

    /** Releases the player, saving the outgoing position. Safe while creation is still running. */
    fun release() {
        creationJob?.cancel()
        creationJob = null
        tickerJob?.cancel()
        tickerJob = null
        cancelAutoNext()
        _player.value?.let { current ->
            saveNow(current)
            current.release()
            PlayerInstances.active.decrementAndGet()
        }
        _player.value = null
    }

    fun next() = switch { it.next() }
    fun prev() = switch { it.prev() }

    /** Jumps to the queued entry the countdown offered. */
    fun startOfferedNext() {
        cancelAutoNext()
        switch { it.next() }
    }

    fun cancelAutoNext() {
        autoNextJob?.cancel()
        autoNextJob = null
        _state.update { it.copy(autoNext = null) }
    }

    fun clearResumeHint() {
        if (_state.value.resumeHintMs != null) _state.update { it.copy(resumeHintMs = null) }
    }

    /** Applies a playback speed chosen in the UI and keeps it for future sessions. */
    fun setPlaybackSpeed(rate: Float) {
        _player.value?.setPlaybackSpeed(rate)
        _state.update { it.copy(playbackRate = rate) }
        viewModelScope.launch { preferences.setPlaybackRate(rate) }
    }

    @OptIn(UnstableApi::class)
    private suspend fun buildPlayer(context: Context): ExoPlayer? {
        val owner = session ?: run {
            _state.update { it.copy(error = R.string.video_unavailable) }
            return null
        }
        val rate = preferences.playbackRate.first()
        val entry = queue.current
        _state.update { it.copy(playbackRate = rate) }
        prepareEntryState(entry)
        val created = ExoPlayer.Builder(context).build()
        PlayerInstances.active.incrementAndGet()
        created.setAudioAttributes(
            AudioAttributes.Builder().setUsage(C.USAGE_MEDIA).setContentType(C.AUDIO_CONTENT_TYPE_MOVIE).build(),
            /* handleAudioFocus = */ true,
        )
        created.setHandleAudioBecomingNoisy(true)
        created.setPlaybackSpeed(rate)
        created.addListener(listener)
        created.setMediaSource(sourceFor(owner, entry), startMs)
        created.prepare()
        created.playWhenReady = playWhenReady
        tickerJob = viewModelScope.launch {
            while (isActive) {
                delay(SAVE_INTERVAL_MS)
                saveNow(created)
            }
        }
        return created
    }

    /** Reads the stored position for [entry], publishes the label and hint, and records [startMs]. */
    private suspend fun prepareEntryState(entry: PlayerQueue.Entry) {
        val saved = watchProgress.savedPosition(entry.id)
        startMs = saved ?: 0L
        _state.update {
            it.copy(
                currentLabel = entry.label,
                hasNext = queue.hasNext,
                hasPrev = queue.hasPrev,
                error = null,
                firstFrameRendered = false,
                lastSeekPositionMs = null,
                autoNext = null,
                resumeHintMs = saved?.takeIf { position -> position >= WatchProgressPolicy.MIN_POSITION_MS },
            )
        }
    }

    @OptIn(UnstableApi::class)
    private fun sourceFor(owner: Session, entry: PlayerQueue.Entry): ProgressiveMediaSource =
        ProgressiveMediaSource.Factory(OkHttpDataSource.Factory(owner.streamClient))
            .createMediaSource(MediaItem.fromUri(owner.resolve("api/items/${entry.id}/stream").toString()))

    private val listener = object : Player.Listener {
        override fun onRenderedFirstFrame() {
            _state.update { it.copy(firstFrameRendered = true) }
        }

        override fun onPositionDiscontinuity(old: Player.PositionInfo, new: Player.PositionInfo, reason: Int) {
            if (reason == Player.DISCONTINUITY_REASON_SEEK) {
                _state.update { it.copy(lastSeekPositionMs = new.positionMs) }
                _player.value?.let(::saveNow)
            }
        }

        override fun onPlaybackStateChanged(playbackState: Int) {
            _state.update { it.copy(buffering = playbackState == Player.STATE_BUFFERING) }
            val current = _player.value ?: return
            if (playbackState == Player.STATE_READY) {
                val duration = current.duration
                // The saved record described a longer file than the one behind the item now.
                if (duration > 0 && startMs > duration) {
                    current.seekTo(0)
                    startMs = 0
                    _state.update { it.copy(resumeHintMs = null) }
                    val entry = queue.current
                    viewModelScope.launch { watchProgress.clear(entry.id) }
                }
            }
            if (playbackState == Player.STATE_ENDED) onEnded()
        }

        override fun onIsPlayingChanged(isPlaying: Boolean) {
            if (!isPlaying) _player.value?.let(::saveNow)
        }

        override fun onPlayerError(error: PlaybackException) {
            val status = generateSequence<Throwable>(error) { it.cause }
                .filterIsInstance<HttpDataSource.InvalidResponseCodeException>()
                .firstOrNull()?.responseCode
            if (status == 401) {
                val owner = session
                if (owner != null) sessions.onUnauthorized(owner)
            } else {
                _state.update { it.copy(error = playbackErrorMessage(status, error)) }
            }
        }
    }

    private fun onEnded() {
        val entry = queue.current
        viewModelScope.launch { watchProgress.clear(entry.id) }
        val upcoming = queue.peekNext() ?: return
        _state.update { it.copy(hasNext = queue.hasNext) }
        autoNextJob = viewModelScope.launch {
            for (seconds in AUTO_NEXT_SECONDS downTo 1) {
                _state.update { it.copy(autoNext = AutoNextState(upcoming.label, seconds)) }
                delay(1_000)
            }
            switch { it.next() }
        }
    }

    /** Advances or rewinds the queue after saving the outgoing entry. */
    @OptIn(UnstableApi::class)
    private fun switch(move: (PlayerQueue) -> PlayerQueue.Entry?) {
        cancelAutoNext()
        val current = _player.value ?: return
        val owner = session ?: return
        saveNow(current)
        val target = move(queue) ?: return
        playWhenReady = true
        viewModelScope.launch {
            prepareEntryState(target)
            current.setMediaSource(sourceFor(owner, target), startMs)
            current.prepare()
            current.playWhenReady = playWhenReady
        }
    }

    /** Snapshots the queue entry at call time so late writes still land on the right item. */
    private fun saveNow(current: ExoPlayer) {
        val entry = queue.current
        val duration = current.duration.takeIf { it != C.TIME_UNSET }?.coerceAtLeast(0) ?: 0
        val position = current.currentPosition.coerceAtLeast(0)
        viewModelScope.launch { watchProgress.update(entry.id, entry.animeId, position, duration) }
    }
}

@StringRes
fun playbackErrorMessage(status: Int?, error: PlaybackException): Int = when {
    status == 404 -> R.string.video_unavailable
    status != null -> R.string.playback_http_failed
    error.errorCode == PlaybackException.ERROR_CODE_DECODING_FAILED ||
        error.errorCode == PlaybackException.ERROR_CODE_DECODING_FORMAT_UNSUPPORTED -> R.string.playback_format_failed
    else -> R.string.playback_failed
}

/** Counts live ExoPlayer instances so tests can assert that leaving the screen releases them. */
@VisibleForTesting
object PlayerInstances {
    val active = AtomicInteger()
}

private const val SAVE_INTERVAL_MS = 3_000L
private const val AUTO_NEXT_SECONDS = 5
