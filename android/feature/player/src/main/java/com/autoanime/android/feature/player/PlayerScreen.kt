package com.autoanime.android.feature.player

import android.app.Activity
import androidx.annotation.OptIn
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LocalContentColor
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.SemanticsPropertyKey
import androidx.compose.ui.semantics.SemanticsPropertyReceiver
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.LifecycleStartEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.media3.common.util.ExperimentalApi
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.ui.compose.material3.Player
import androidx.media3.ui.compose.material3.PlayerDefaults
import androidx.media3.ui.compose.material3.indicator.ProgressSlider
import com.autoanime.android.core.designsystem.AppIcons
import com.autoanime.android.core.designsystem.AutoAnimeTheme
import kotlinx.coroutines.delay

@Composable
fun PlayerRoute(itemId: Long, title: String, onClose: () -> Unit) {
    val viewModel = hiltViewModel<PlayerViewModel, PlayerViewModel.Factory>(
        key = "item-$itemId",
        creationCallback = { it.create(itemId) },
    )
    val state by viewModel.state.collectAsStateWithLifecycle()
    val context = LocalContext.current
    var player by remember { mutableStateOf<ExoPlayer?>(null) }
    LifecycleStartEffect(viewModel) {
        val created = viewModel.createPlayer(context)
        player = created
        onStopOrDispose {
            player = null
            created?.let(viewModel::release)
        }
    }
    ImmersiveMode()
    // Video chrome stays dark in both app themes: Media3's bottom scrim uses the background color
    // and its icons and time text use the content color, which is not set outside a Surface.
    AutoAnimeTheme(darkTheme = true) {
        CompositionLocalProvider(LocalContentColor provides Color.White) {
            PlayerScreen(player, title, state, onClose)
        }
    }
}

@OptIn(UnstableApi::class, ExperimentalApi::class)
@Composable
fun PlayerScreen(player: ExoPlayer?, title: String, state: PlayerUiState, onClose: () -> Unit) {
    var controlsVisible by remember { mutableStateOf(true) }
    LaunchedEffect(controlsVisible) {
        if (controlsVisible) {
            delay(CONTROLS_TIMEOUT_MS)
            if (player?.isPlaying == true) controlsVisible = false
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
                .clickable(interactionSource = null, indication = null) { controlsVisible = !controlsVisible },
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
                        Text(
                            title,
                            color = Color.White,
                            style = MaterialTheme.typography.titleMedium,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                        )
                    }
                }
            },
            // Only one item is queued, so previous/next would have nothing to skip to.
            centerControls = { current, visible ->
                PlayerDefaults.CenterControls(
                    current,
                    visible,
                    Modifier.fillMaxWidth(),
                    backSecondary = {},
                    forwardSecondary = {},
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
        state.error?.let {
            Text(
                stringResource(it),
                color = Color.White,
                style = MaterialTheme.typography.bodyLarge,
                modifier = Modifier.align(Alignment.Center).padding(24.dp),
            )
        }
    }
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

private val TopScrim = Brush.verticalGradient(listOf(Color.Black.copy(alpha = 0.6f), Color.Transparent))

object PlayerTestTags {
    const val PLAYER = "player"
    const val CLOSE = "close"
    const val SEEK_BAR = "seek_bar"
}

/** Test-only semantics; not announced by accessibility services. */
object PlayerSemantics {
    val FirstFrameRendered = SemanticsPropertyKey<Boolean>("FirstFrameRendered")
    val LastSeekPositionMs = SemanticsPropertyKey<Long>("LastSeekPositionMs")
}

private var SemanticsPropertyReceiver.firstFrameRendered by PlayerSemantics.FirstFrameRendered
private var SemanticsPropertyReceiver.lastSeekPositionMs by PlayerSemantics.LastSeekPositionMs
