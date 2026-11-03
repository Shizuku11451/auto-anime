package com.autoanime.android.feature.player

import android.content.Context
import androidx.annotation.OptIn
import androidx.annotation.StringRes
import androidx.annotation.VisibleForTesting
import androidx.lifecycle.ViewModel
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
import com.autoanime.android.core.data.Session
import com.autoanime.android.core.data.SessionRepository
import com.autoanime.android.core.data.SessionState
import dagger.assisted.Assisted
import dagger.assisted.AssistedFactory
import dagger.assisted.AssistedInject
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import java.util.concurrent.atomic.AtomicInteger

data class PlayerUiState(
    @StringRes val error: Int? = null,
    val firstFrameRendered: Boolean = false,
    val lastSeekPositionMs: Long? = null,
)

/**
 * Keeps the playback position across player instances. The ExoPlayer itself only lives while
 * the screen is started, so leaving the app releases the decoder and network connection.
 */
@HiltViewModel(assistedFactory = PlayerViewModel.Factory::class)
class PlayerViewModel @AssistedInject constructor(
    @Assisted private val itemId: Long,
    private val sessions: SessionRepository,
) : ViewModel() {
    @AssistedFactory
    interface Factory {
        fun create(itemId: Long): PlayerViewModel
    }

    private val _state = MutableStateFlow(PlayerUiState())
    val state: StateFlow<PlayerUiState> = _state.asStateFlow()

    private val session: Session? = (sessions.state.value as? SessionState.Connected)?.session
    private var position = 0L
    private var playWhenReady = true

    @OptIn(UnstableApi::class)
    fun createPlayer(context: Context): ExoPlayer? {
        val owner = session ?: run {
            _state.update { it.copy(error = R.string.video_unavailable) }
            return null
        }
        val player = ExoPlayer.Builder(context).build()
        PlayerInstances.active.incrementAndGet()
        player.setAudioAttributes(
            AudioAttributes.Builder().setUsage(C.USAGE_MEDIA).setContentType(C.AUDIO_CONTENT_TYPE_MOVIE).build(),
            /* handleAudioFocus = */ true,
        )
        player.setHandleAudioBecomingNoisy(true)
        player.addListener(object : Player.Listener {
            override fun onRenderedFirstFrame() {
                _state.update { it.copy(firstFrameRendered = true) }
            }

            override fun onPositionDiscontinuity(old: Player.PositionInfo, new: Player.PositionInfo, reason: Int) {
                if (reason == Player.DISCONTINUITY_REASON_SEEK) _state.update { it.copy(lastSeekPositionMs = new.positionMs) }
            }

            override fun onPlayerError(error: PlaybackException) {
                val status = generateSequence<Throwable>(error) { it.cause }
                    .filterIsInstance<HttpDataSource.InvalidResponseCodeException>()
                    .firstOrNull()?.responseCode
                if (status == 401) sessions.onUnauthorized(owner)
                else _state.update { it.copy(error = playbackErrorMessage(status, error)) }
            }
        })
        val source = ProgressiveMediaSource.Factory(OkHttpDataSource.Factory(owner.streamClient))
            .createMediaSource(MediaItem.fromUri(owner.resolve("api/items/$itemId/stream").toString()))
        player.setMediaSource(source, position)
        player.prepare()
        player.playWhenReady = playWhenReady
        return player
    }

    fun release(player: ExoPlayer) {
        position = player.currentPosition.coerceAtLeast(0)
        playWhenReady = player.playWhenReady
        player.release()
        PlayerInstances.active.decrementAndGet()
    }
}

@StringRes
fun playbackErrorMessage(status: Int?, error: PlaybackException): Int = when {
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
