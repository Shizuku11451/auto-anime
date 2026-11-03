package com.autoanime.android.feature.library

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.autoanime.android.core.designsystem.AppIcons
import com.autoanime.android.core.designsystem.CoverImage
import com.autoanime.android.core.designsystem.ErrorState
import com.autoanime.android.core.designsystem.LoadingState
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Switch
import androidx.compose.material3.TextButton
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.lifecycle.compose.LifecycleStartEffect
import com.autoanime.android.core.designsystem.message
import com.autoanime.android.core.model.CorrectInput
import com.autoanime.android.core.model.MediaItem
import com.autoanime.android.core.model.isSaved
import com.autoanime.android.core.designsystem.R as DesignR

@Composable
fun DetailRoute(
    animeId: Long,
    onPlay: (itemId: Long, title: String) -> Unit,
    onBack: (() -> Unit)?,
    onEdit: (animeId: Long) -> Unit = {},
    onBind: (animeId: Long) -> Unit = {},
) {
    val viewModel = hiltViewModel<DetailViewModel, DetailViewModel.Factory>(
        key = "anime-$animeId",
        creationCallback = { it.create(animeId) },
    )
    val state by viewModel.state.collectAsStateWithLifecycle()
    // The first start is covered by the view model's own load; later ones follow edits elsewhere.
    var started by remember { mutableStateOf(false) }
    LifecycleStartEffect(viewModel) {
        if (started) viewModel.load() else started = true
        onStopOrDispose { }
    }
    val snackbar = remember { SnackbarHostState() }
    val resources = LocalResources.current
    LaunchedEffect(viewModel) {
        viewModel.messages.collect { message ->
            snackbar.showSnackbar(
                when (message) {
                    is DetailMessage.Corrected -> message.warning.ifBlank {
                        resources.getString(if (message.moved) R.string.corrected_moved else R.string.corrected)
                    }
                    DetailMessage.RematchQueued -> resources.getString(R.string.rematch_queued)
                    is DetailMessage.Failed -> message.error.message(resources)
                },
            )
        }
    }
    DetailScreen(
        state = state,
        onPlay = onPlay,
        onBack = onBack,
        onRetry = viewModel::load,
        snackbar = snackbar,
        onEdit = { onEdit(animeId) },
        onBind = { onBind(animeId) },
        onRematch = viewModel::rematch,
        onCorrect = viewModel::correct,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DetailScreen(
    state: DetailUiState,
    onPlay: (itemId: Long, title: String) -> Unit,
    onBack: (() -> Unit)?,
    onRetry: () -> Unit,
    snackbar: SnackbarHostState = remember { SnackbarHostState() },
    onEdit: () -> Unit = {},
    onBind: () -> Unit = {},
    onRematch: () -> Unit = {},
    onCorrect: (MediaItem, CorrectInput) -> Unit = { _, _ -> },
) {
    var menu by remember { mutableStateOf(false) }
    var correcting by remember { mutableStateOf<MediaItem?>(null) }
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(state.title, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                navigationIcon = {
                    if (onBack != null) IconButton(onClick = onBack) {
                        Icon(painterResource(AppIcons.Back), contentDescription = stringResource(DesignR.string.back))
                    }
                },
                actions = {
                    if (state.working) CircularProgressIndicator(Modifier.padding(12.dp).size(24.dp), strokeWidth = 2.dp)
                    if (state.loaded) Box {
                        IconButton(onClick = { menu = true }, modifier = Modifier.testTag(LibraryTestTags.DETAIL_MENU)) {
                            Icon(painterResource(AppIcons.More), contentDescription = stringResource(R.string.more))
                        }
                        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                            DropdownMenuItem(text = { Text(stringResource(R.string.edit_anime)) }, onClick = { menu = false; onEdit() })
                            DropdownMenuItem(text = { Text(stringResource(R.string.bind_metadata)) }, onClick = { menu = false; onBind() })
                            DropdownMenuItem(
                                text = { Text(stringResource(R.string.rematch)) },
                                enabled = !state.working && !state.bound,
                                onClick = { menu = false; onRematch() },
                            )
                        }
                    }
                },
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { padding ->
        val body = Modifier.fillMaxSize().padding(padding)
        when {
            !state.loaded && state.error != null -> ErrorState(state.error, onRetry, body)
            !state.loaded -> LoadingState(body)
            else -> LazyColumn(body, contentPadding = PaddingValues(bottom = 24.dp)) {
                item(contentType = "header") { Header(state) }
                if (state.episodes.isEmpty()) item(contentType = "empty") {
                    Text(
                        stringResource(R.string.no_episodes),
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(16.dp),
                    )
                }
                items(state.episodes, key = { it.id }, contentType = { "episode" }) { item ->
                    val label = item.label()
                    val details = listOf(item.fansub_group, item.quality, if (item.isSaved) "" else item.status)
                        .filter(String::isNotBlank).joinToString(" · ")
                    ListItem(
                        headlineContent = { Text(label, maxLines = 2, overflow = TextOverflow.Ellipsis) },
                        supportingContent = details.takeIf(String::isNotEmpty)?.let { { Text(it) } },
                        leadingContent = if (item.isSaved) ({ Icon(painterResource(AppIcons.Play), contentDescription = null) }) else null,
                        trailingContent = {
                            IconButton(
                                onClick = { correcting = item },
                                enabled = !state.working,
                                modifier = Modifier.testTag(LibraryTestTags.correct(item.id)),
                            ) {
                                Icon(painterResource(AppIcons.Edit), contentDescription = stringResource(R.string.correct_item))
                            }
                        },
                        modifier = if (item.isSaved) Modifier.clickable { onPlay(item.id, label) } else Modifier,
                    )
                }
            }
        }
    }
    correcting?.let { item ->
        CorrectDialog(item, state.title, onDismiss = { correcting = null }, onSave = { input -> correcting = null; onCorrect(item, input) })
    }
}

@Composable
private fun Header(state: DetailUiState) {
    Row(Modifier.fillMaxWidth().padding(16.dp), horizontalArrangement = Arrangement.spacedBy(16.dp)) {
        CoverImage(
            url = state.coverUrl,
            contentDescription = stringResource(R.string.anime_cover),
            modifier = Modifier.width(120.dp).aspectRatio(2f / 3f).clip(RoundedCornerShape(12.dp)),
        )
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(state.title, style = MaterialTheme.typography.titleLarge)
            if (state.summary.isNotBlank()) Text(
                state.summary,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 8,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

@Composable
private fun MediaItem.label(): String = title.ifBlank { stringResource(R.string.episode_number, episode) }

@Composable
fun DetailPlaceholder() {
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Text(stringResource(R.string.select_anime), color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

/**
 * Moves one release to another anime, season or episode. Moving saved media is a separate,
 * explicit choice: the server migrates the file with rollback; otherwise only the record moves.
 */
@Composable
private fun CorrectDialog(item: MediaItem, currentTitle: String, onDismiss: () -> Unit, onSave: (CorrectInput) -> Unit) {
    var otherTitle by remember { mutableStateOf("") }
    var season by remember { mutableStateOf(item.season.toString()) }
    var episode by remember { mutableStateOf(item.episode) }
    var rearchive by remember { mutableStateOf(false) }
    val seasonValue = season.trim().toIntOrNull()?.takeIf { it in 0..999 }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.correct_item)) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(item.title, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                OutlinedTextField(
                    value = otherTitle,
                    onValueChange = { otherTitle = it },
                    label = { Text(stringResource(R.string.move_to_anime)) },
                    placeholder = { Text(currentTitle) },
                    singleLine = true,
                )
                OutlinedTextField(
                    value = season,
                    onValueChange = { season = it.filter(Char::isDigit).take(3) },
                    label = { Text(stringResource(R.string.item_season)) },
                    supportingText = {
                        Text(
                            when {
                                seasonValue == null -> stringResource(R.string.item_season_invalid)
                                item.season_auto > 0 -> stringResource(R.string.item_season_hint_parsed, item.season_auto)
                                else -> stringResource(R.string.item_season_hint)
                            },
                        )
                    },
                    isError = seasonValue == null,
                    singleLine = true,
                )
                OutlinedTextField(
                    value = episode,
                    onValueChange = { episode = it },
                    label = { Text(stringResource(R.string.item_episode)) },
                    singleLine = true,
                )
                if (item.isSaved) Row(
                    Modifier.fillMaxWidth().toggleable(rearchive, role = Role.Switch, onValueChange = { rearchive = it }),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Column(Modifier.weight(1f)) {
                        Text(stringResource(R.string.rearchive))
                        Text(
                            stringResource(R.string.rearchive_hint),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    Switch(checked = rearchive, onCheckedChange = null)
                }
            }
        },
        confirmButton = {
            TextButton(
                enabled = seasonValue != null,
                onClick = {
                    val title = otherTitle.trim()
                    onSave(
                        CorrectInput(
                            season = seasonValue ?: return@TextButton,
                            episode = episode.trim(),
                            rearchive = rearchive,
                            anime_title = title.ifEmpty { null },
                            anime_id = if (title.isEmpty()) item.anime_id.takeIf { it > 0 } else null,
                        ),
                    )
                },
                modifier = Modifier.testTag(LibraryTestTags.SAVE_CORRECTION),
            ) { Text(stringResource(R.string.save)) }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}
