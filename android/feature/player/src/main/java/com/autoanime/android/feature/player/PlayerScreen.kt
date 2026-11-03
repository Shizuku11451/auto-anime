package com.autoanime.android.feature.player

import android.app.Activity
import android.content.Context
import android.media.AudioManager
import android.view.Window
import androidx.annotation.OptIn
import androidx.annotation.StringRes
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectHorizontalDragGestures
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.gestures.detectVerticalDragGestures
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LocalContentColor
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.SemanticsPropertyKey
import androidx.compose.ui.semantics.SemanticsPropertyReceiver
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.LifecycleStartEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.media3.common.C
import androidx.media3.common.Tracks
import androidx.media3.common.TrackSelectionOverride
import androidx.media3.common.text.Cue
import androidx.media3.common.util.ExperimentalApi
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.ui.SubtitleView
import androidx.media3.ui.compose.material3.Player
import androidx.media3.ui.compose.material3.PlayerDefaults
import androidx.media3.ui.compose.material3.indicator.ProgressSlider
import com.autoanime.android.core.designsystem.AppIcons
import com.autoanime.android.core.designsystem.AutoAnimeTheme
import com.autoanime.android.core.model.PlayableEpisode
import kotlinx.coroutines.delay
import java.util.Locale
import kotlin.math.roundToInt

@Composable
fun PlayerRoute(episodes: List<PlayableEpisode>, startIndex: Int, title: String, onClose: () -> Unit) {
    val viewModel = hiltViewModel<PlayerViewModel, PlayerViewModel.Factory>(
        key = "player-${episodes.firstOrNull()?.id}",
        creationCallback = { it.create(episodes, startIndex) },
    )
    val state by viewModel.state.collectAsStateWithLifecycle()
    val player by viewModel.player.collectAsStateWithLifecycle()
    val context = LocalContext.current
    LifecycleStartEffect(viewModel) {
        viewModel.createPlayer(context)
        onStopOrDispose { viewModel.release() }
    }
    ImmersiveMode()
    // Video chrome stays dark in both app themes: Media3's bottom scrim uses the background color
    // and its icons and time text use the content color, which is not set outside a Surface.
    AutoAnimeTheme(darkTheme = true) {
        CompositionLocalProvider(LocalContentColor provides Color.White) {
            PlayerScreen(player, title, state, viewModel, onClose)
        }
    }
}

@OptIn(UnstableApi::class, ExperimentalApi::class)
@Composable
fun PlayerScreen(
    player: ExoPlayer?,
    title: String,
    state: PlayerUiState,
    viewModel: PlayerViewModel,
    onClose: () -> Unit,
) {
    val view = LocalView.current
    val context = LocalContext.current
    var controlsVisible by remember { mutableStateOf(true) }
    var skipHint by remember { mutableStateOf<Int?>(null) }
    var seekPreviewMs by remember { mutableStateOf<Long?>(null) }
    var gestureHint by remember { mutableStateOf<GestureHint?>(null) }
    var showTracks by remember { mutableStateOf(false) }
    var showSpeed by remember { mutableStateOf(false) }

    LaunchedEffect(controlsVisible) {
        if (controlsVisible) {
            delay(CONTROLS_TIMEOUT_MS)
            if (player?.isPlaying == true) controlsVisible = false
        }
    }
    LaunchedEffect(state.resumeHintMs) {
        if (state.resumeHintMs != null) {
            delay(RESUME_HINT_TIMEOUT_MS)
            viewModel.clearResumeHint()
        }
    }
    LaunchedEffect(skipHint) {
        if (skipHint != null) {
            delay(HINT_TIMEOUT_MS)
            skipHint = null
        }
    }
    LaunchedEffect(gestureHint) {
        if (gestureHint != null) {
            delay(HINT_TIMEOUT_MS)
            gestureHint = null
        }
    }
    Box(
        Modifier
            .fillMaxSize()
            .background(Color.Black)
            .testTag(PlayerTestTags.PLAYER)
            .semantics {
                firstFrameRendered = state.firstFrameRendered
                state.lastSeekPositionMs?.let { lastSeekPositionMs = it }
            },
    ) {
        Player(
            player = player,
            showControls = controlsVisible,
            modifier = Modifier
                .fillMaxSize()
                .pointerInput(player) {
                    detectTapGestures(
                        onTap = { controlsVisible = !controlsVisible },
                        onDoubleTap = { offset ->
                            val current = player ?: return@detectTapGestures
                            when {
                                offset.x < size.width / 3f -> {
                                    current.seekTo((current.currentPosition - SKIP_MS).coerceAtLeast(0))
                                    skipHint = R.string.skip_back
                                }
                                offset.x > size.width * 2f / 3f -> {
                                    current.seekTo(current.currentPosition + SKIP_MS)
                                    skipHint = R.string.skip_forward
                                }
                                else -> if (current.isPlaying) current.pause() else current.play()
                            }
                        },
                    )
                }
                .pointerInput(player) {
                    detectHorizontalDragGestures(
                        onDragStart = { seekPreviewMs = player?.currentPosition?.coerceAtLeast(0) ?: 0 },
                        onDragEnd = {
                            seekPreviewMs?.let { target -> player?.seekTo(target) }
                            seekPreviewMs = null
                        },
                        onDragCancel = { seekPreviewMs = null },
                    ) { change, dragAmount ->
                        change.consume()
                        val current = player ?: return@detectHorizontalDragGestures
                        val duration = current.duration.takeIf { it != C.TIME_UNSET } ?: return@detectHorizontalDragGestures
                        val base = seekPreviewMs ?: current.currentPosition.coerceAtLeast(0)
                        seekPreviewMs = (base + (dragAmount / size.width * SEEK_DRAG_RANGE_MS).toLong())
                            .coerceIn(0, duration)
                    }
                }
                .pointerInput(player, context) {
                    var leftSide = true
                    var brightnessBase = 0.5f
                    var volumeBase = 0
                    var volumeMax = 1
                    val audio = context.getSystemService(Context.AUDIO_SERVICE) as AudioManager
                    detectVerticalDragGestures(
                        onDragStart = { offset ->
                            leftSide = offset.x < size.width / 2f
                            if (leftSide) {
                                brightnessBase = (view.context as? Activity)?.window
                                    ?.attributes?.screenBrightness
                                    ?.takeIf { it in 0.01f..1f } ?: 0.5f
                            } else {
                                volumeBase = audio.getStreamVolume(AudioManager.STREAM_MUSIC)
                                volumeMax = audio.getStreamMaxVolume(AudioManager.STREAM_MUSIC)
                            }
                        },
                        onDragEnd = { },
                        onDragCancel = { },
                    ) { change, dragAmount ->
                        change.consume()
                        val fraction = dragAmount / size.height
                        gestureHint = if (leftSide) {
                            adjustBrightness(view, brightnessBase, fraction)
                        } else {
                            adjustVolume(audio, volumeBase, volumeMax, fraction)
                        }
                    }
                },
            topControls = { _, visible ->
                AnimatedVisibility(visible, enter = fadeIn(), exit = fadeOut()) {
                    Row(
                        Modifier
                            .fillMaxWidth()
                            .background(TopScrim)
                            .safeDrawingPadding()
                            // Extra bottom space lets the scrim fade out below the title.
                            .padding(start = 4.dp, top = 4.dp, end = 4.dp, bottom = 24.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        IconButton(onClick = onClose, modifier = Modifier.testTag(PlayerTestTags.CLOSE)) {
                            Icon(painterResource(AppIcons.Close), stringResource(R.string.close_player), tint = Color.White)
                        }
                        val header = if (state.currentLabel.isBlank()) title else "$title · ${state.currentLabel}"
                        Text(
                            header,
                            color = Color.White,
                            style = MaterialTheme.typography.titleMedium,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            modifier = Modifier.weight(1f),
                        )
                        TextButton(onClick = { showSpeed = true }, modifier = Modifier.testTag(PlayerTestTags.SPEED)) {
                            Text(speedLabel(state.playbackRate), color = Color.White)
                        }
                        TextButton(onClick = { showTracks = true }, modifier = Modifier.testTag(PlayerTestTags.TRACKS)) {
                            Text(stringResource(R.string.audio_and_subtitles), color = Color.White)
                        }
                    }
                }
            },
            centerControls = { current, visible ->
                PlayerDefaults.CenterControls(
                    current,
                    visible,
                    Modifier.fillMaxWidth(),
                    backSecondary = {
                        IconButton(onClick = viewModel::prev, enabled = state.hasPrev) {
                            Icon(painterResource(AppIcons.Back), stringResource(R.string.previous_episode))
                        }
                    },
                    forwardSecondary = {
                        IconButton(onClick = viewModel::next, enabled = state.hasNext) {
                            Icon(
                                painterResource(AppIcons.Back),
                                stringResource(R.string.next_episode),
                                modifier = Modifier.graphicsLayer { scaleX = -1f },
                            )
                        }
                    },
                )
            },
            bottomControls = { current, visible ->
                PlayerDefaults.BottomControls(
                    current,
                    visible,
                    modifier = Modifier.safeDrawingPadding(),
                    progressSlider = { ProgressSlider(it, Modifier.testTag(PlayerTestTags.SEEK_BAR)) },
                )
            },
        )
        SubtitleOverlay(player)

        if (state.buffering && state.error == null && !state.firstFrameRendered) {
            CircularProgressIndicator(Modifier.align(Alignment.Center))
        }
        state.error?.let {
            Text(
                stringResource(it),
                color = Color.White,
                style = MaterialTheme.typography.bodyLarge,
                textAlign = TextAlign.Center,
                modifier = Modifier.align(Alignment.Center).padding(24.dp),
            )
        }
        state.resumeHintMs?.let {
            HintBanner(
                stringResource(R.string.resume_hint, formatTime(it)),
                Modifier.align(Alignment.BottomCenter).testTag(PlayerTestTags.RESUME_HINT),
            )
        }
        skipHint?.let { HintBanner(stringResource(it), Modifier.align(Alignment.Center)) }
        seekPreviewMs?.let { target ->
            val duration = player?.duration?.takeIf { it != C.TIME_UNSET } ?: 0
            HintBanner("${formatTime(target)} / ${formatTime(duration)}", Modifier.align(Alignment.Center))
        }
        gestureHint?.let { HintBanner("${stringResource(it.label)} ${it.percent}%", Modifier.align(Alignment.Center)) }

        state.autoNext?.let { next ->
            Surface(
                shape = MaterialTheme.shapes.medium,
                color = Color.Black.copy(alpha = 0.8f),
                modifier = Modifier
                    .align(Alignment.Center)
                    .testTag(PlayerTestTags.AUTO_NEXT),
            ) {
                Column(Modifier.padding(20.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                    Text(stringResource(R.string.auto_next_title), style = MaterialTheme.typography.labelLarge)
                    Text(
                        next.label,
                        style = MaterialTheme.typography.titleMedium,
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis,
                        textAlign = TextAlign.Center,
                        modifier = Modifier.padding(vertical = 6.dp),
                    )
                    Text("${next.secondsLeft}", style = MaterialTheme.typography.titleLarge)
                    Row {
                        TextButton(onClick = viewModel::startOfferedNext, modifier = Modifier.testTag(PlayerTestTags.PLAY_NOW)) {
                            Text(stringResource(R.string.play_now))
                        }
                        TextButton(onClick = viewModel::cancelAutoNext) { Text(stringResource(R.string.cancel)) }
                    }
                }
            }
        }
    }
    if (showSpeed) {
        SpeedDialog(
            current = state.playbackRate,
            onSelect = { rate ->
                showSpeed = false
                viewModel.setPlaybackSpeed(rate)
            },
            onDismiss = { showSpeed = false },
        )
    }
    if (showTracks) {
        TrackSheet(player = player, onDismiss = { showTracks = false })
    }
}

/** Renders the player's selected text cues (SRT, and ASS/SSA without effects) over the video. */
@OptIn(UnstableApi::class)
@Composable
private fun SubtitleOverlay(player: ExoPlayer?) {
    if (player == null) return
    val cues by produceState<List<Cue>>(emptyList(), player) {
        val listener = object : androidx.media3.common.Player.Listener {
            override fun onCues(value: List<Cue>) {
                this@produceState.value = value
            }
        }
        player.addListener(listener)
        awaitDispose { player.removeListener(listener) }
    }
    AndroidView(
        factory = { context -> SubtitleView(context, null) },
        update = { view -> view.setCues(cues) },
        modifier = Modifier.fillMaxSize(),
    )
}

/** A short-lived banner over the video for seek, gesture and resume feedback. */
@Composable
private fun HintBanner(text: String, modifier: Modifier = Modifier) {
    Surface(shape = MaterialTheme.shapes.small, color = Color.Black.copy(alpha = 0.7f), modifier = modifier.padding(24.dp)) {
        Text(
            text,
            color = Color.White,
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.padding(horizontal = 14.dp, vertical = 8.dp),
        )
    }
}

@Composable
private fun SpeedDialog(current: Float, onSelect: (Float) -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.playback_speed)) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState())) {
                RATES.forEach { rate ->
                    TextButton(onClick = { onSelect(rate) }, enabled = rate != current) {
                        Text(
                            speedLabel(rate),
                            color = if (rate == current) MaterialTheme.colorScheme.primary else LocalContentColor.current,
                        )
                    }
                }
            }
        },
        confirmButton = {},
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}

@OptIn(UnstableApi::class)
@Composable
private fun TrackSheet(player: ExoPlayer?, onDismiss: () -> Unit) {
    if (player == null) return
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.audio_and_subtitles)) },
        text = {
        val tracks by produceState(player.currentTracks, player) {
            val listener = object : androidx.media3.common.Player.Listener {
                override fun onTracksChanged(tracks: Tracks) {
                    value = tracks
                }
            }
            player.addListener(listener)
            awaitDispose { player.removeListener(listener) }
        }
        Column(
            Modifier
                .fillMaxWidth()
                .verticalScroll(rememberScrollState()),
        ) {
            Text(stringResource(R.string.audio_tracks), style = MaterialTheme.typography.titleSmall)
            val audio = trackOptions(tracks, C.TRACK_TYPE_AUDIO)
            if (audio.isEmpty()) Text(stringResource(R.string.no_tracks), style = MaterialTheme.typography.bodyMedium)
            audio.forEach { option ->
                TextButton(onClick = { option.select(player) }, enabled = !option.selected) {
                    Text(
                        option.label,
                        color = if (option.selected) MaterialTheme.colorScheme.primary else LocalContentColor.current,
                    )
                }
            }
            Text(
                stringResource(R.string.subtitle_tracks),
                style = MaterialTheme.typography.titleSmall,
                modifier = Modifier.padding(top = 12.dp),
            )
            val subtitlesDisabled = C.TRACK_TYPE_TEXT in player.trackSelectionParameters.disabledTrackTypes
            TextButton(onClick = { player.disableSubtitles() }, enabled = !subtitlesDisabled) {
                Text(
                    stringResource(R.string.subtitles_off),
                    color = if (subtitlesDisabled) MaterialTheme.colorScheme.primary else LocalContentColor.current,
                )
            }
            trackOptions(tracks, C.TRACK_TYPE_TEXT).forEach { option ->
                TextButton(onClick = { option.select(player) }, enabled = !option.selected) {
                    Text(
                        option.label,
                        color = if (option.selected) MaterialTheme.colorScheme.primary else LocalContentColor.current,
                    )
                }
            }
        }
        },
        confirmButton = {},
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}

/** One selectable track with a display label; selecting overrides the player's track selection. */
private data class TrackOption(val group: Tracks.Group, val trackIndex: Int, val label: String, val selected: Boolean) {
    fun select(player: ExoPlayer) {
        player.trackSelectionParameters = player.trackSelectionParameters.buildUpon()
            .setTrackTypeDisabled(group.type, false)
            .setOverrideForType(TrackSelectionOverride(group.mediaTrackGroup, trackIndex))
            .build()
    }
}

@OptIn(UnstableApi::class)
@Composable
private fun trackOptions(tracks: Tracks, type: Int): List<TrackOption> = buildList {
    tracks.groups.forEach { group ->
        if (group.type != type) return@forEach
        for (index in 0 until group.length) {
            if (!group.isTrackSupported(index)) continue
            val format = group.getTrackFormat(index)
            val label = format.label?.takeIf(CharSequence::isNotBlank)
                ?: format.language?.let { Locale(it).displayLanguage }
                ?: stringResource(R.string.track_index, size + 1)
            add(TrackOption(group, index, label, group.isTrackSelected(index)))
        }
    }
}

private fun ExoPlayer.disableSubtitles() {
    trackSelectionParameters = trackSelectionParameters.buildUpon()
        .setTrackTypeDisabled(C.TRACK_TYPE_TEXT, true)
        .build()
}

private data class GestureHint(@StringRes val label: Int, val percent: Int)

/** Left-half vertical drags change the window brightness; returns the hint or null without a window. */
private fun adjustBrightness(view: android.view.View, base: Float, fraction: Float): GestureHint? {
    val window = (view.context as? Activity)?.window ?: return null
    val attributes = window.attributes
    val next = (base - fraction).coerceIn(0.01f, 1f)
    attributes.screenBrightness = next
    window.attributes = attributes
    return GestureHint(R.string.brightness, (next * 100).roundToInt())
}

/** Right-half vertical drags change the media volume; returns the hint. */
private fun adjustVolume(audio: AudioManager, base: Int, max: Int, fraction: Float): GestureHint {
    val next = (base - (fraction * max).roundToInt()).coerceIn(0, max)
    audio.setStreamVolume(AudioManager.STREAM_MUSIC, next, 0)
    return GestureHint(R.string.volume, next * 100 / max)
}

private fun speedLabel(rate: Float): String = if (rate == rate.roundToInt().toFloat()) {
    String.format(Locale.ROOT, "%d.0x", rate.roundToInt())
} else {
    String.format(Locale.ROOT, "%sx", rate)
}

private fun formatTime(ms: Long): String {
    val totalSeconds = ms / 1_000
    val hours = totalSeconds / 3_600
    val minutes = totalSeconds % 3_600 / 60
    val seconds = totalSeconds % 60
    return if (hours > 0) String.format(Locale.ROOT, "%d:%02d:%02d", hours, minutes, seconds)
    else String.format(Locale.ROOT, "%d:%02d", minutes, seconds)
}

/** Hides system bars and keeps the screen on while the player is shown. */
@Composable
private fun ImmersiveMode() {
    val view = LocalView.current
    DisposableEffect(view) {
        val window = (view.context as? Activity)?.window
        val controller = window?.let { WindowCompat.getInsetsController(it, view) }
        controller?.systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
        controller?.hide(WindowInsetsCompat.Type.systemBars())
        view.keepScreenOn = true
        onDispose {
            controller?.show(WindowInsetsCompat.Type.systemBars())
            view.keepScreenOn = false
        }
    }
}

private const val CONTROLS_TIMEOUT_MS = 4_000L
private const val RESUME_HINT_TIMEOUT_MS = 2_600L
private const val HINT_TIMEOUT_MS = 700L
private const val SKIP_MS = 10_000L
/** Full-width horizontal drag equals this much playback. */
private const val SEEK_DRAG_RANGE_MS = 90_000f

private val RATES = listOf(0.5f, 0.75f, 1f, 1.25f, 1.5f, 2f, 3f)

private val TopScrim = Brush.verticalGradient(listOf(Color.Black.copy(alpha = 0.6f), Color.Transparent))

object PlayerTestTags {
    const val PLAYER = "player"
    const val CLOSE = "close"
    const val SEEK_BAR = "seek_bar"
    const val SPEED = "speed"
    const val TRACKS = "audio_subtitles"
    const val RESUME_HINT = "resume_hint"
    const val AUTO_NEXT = "auto_next"
    const val PLAY_NOW = "play_now"
}

/** Test-only semantics; not announced by accessibility services. */
object PlayerSemantics {
    val FirstFrameRendered = SemanticsPropertyKey<Boolean>("FirstFrameRendered")
    val LastSeekPositionMs = SemanticsPropertyKey<Long>("LastSeekPositionMs")
}

private var SemanticsPropertyReceiver.firstFrameRendered by PlayerSemantics.FirstFrameRendered
private var SemanticsPropertyReceiver.lastSeekPositionMs by PlayerSemantics.LastSeekPositionMs
