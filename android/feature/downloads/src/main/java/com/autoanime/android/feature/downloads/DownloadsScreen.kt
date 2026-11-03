package com.autoanime.android.feature.downloads

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.LifecycleStartEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.autoanime.android.core.designsystem.AppIcons
import com.autoanime.android.core.designsystem.CoverImage
import com.autoanime.android.core.designsystem.ErrorState
import com.autoanime.android.core.designsystem.LoadingState
import com.autoanime.android.core.designsystem.LocalStatusColors
import com.autoanime.android.core.designsystem.message
import com.autoanime.android.core.model.DownloadAction
import com.autoanime.android.core.model.STATUS_PAUSED
import com.autoanime.android.core.model.STATUS_DOWNLOADING
import com.autoanime.android.core.model.STATUS_RESUMING
import com.autoanime.android.core.model.SENT_TO_PREFIX
import com.autoanime.android.core.model.STATUS_PENDING
import com.autoanime.android.core.model.STATUS_QUEUED
import com.autoanime.android.core.model.STATUS_QUEUED_RESUME
import com.autoanime.android.core.model.STATUS_FAILED
import com.autoanime.android.core.model.STATUS_SAVED
import com.autoanime.android.core.model.STATUS_WAITING_DOWNLOADER
import com.autoanime.android.core.model.StatusGroup
import com.autoanime.android.core.model.canPause
import com.autoanime.android.core.model.canResume
import com.autoanime.android.core.model.canRetry
import com.autoanime.android.core.model.effectiveStatus
import com.autoanime.android.core.model.isDownloadingStatus
import com.autoanime.android.core.model.PlayableEpisode
import com.autoanime.android.core.model.isPlayable
import com.autoanime.android.core.model.restartsOnResume

@Composable
fun DownloadsRoute(
    onPlay: (episodes: List<PlayableEpisode>, index: Int, title: String) -> Unit,
    viewModel: DownloadsViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    LifecycleStartEffect(viewModel) {
        viewModel.setVisible(true)
        onStopOrDispose { viewModel.setVisible(false) }
    }
    val snackbar = remember { SnackbarHostState() }
    val resources = LocalResources.current
    LaunchedEffect(viewModel) {
        viewModel.messages.collect { message ->
            snackbar.showSnackbar(
                if (message is DownloadMessage.Failed) message.error.message(resources) else resources.getString(message.textRes()),
            )
        }
    }
    DownloadsScreen(
        state = state,
        snackbar = snackbar,
        onGroup = viewModel::selectGroup,
        onQuery = viewModel::setQuery,
        onRefresh = viewModel::refresh,
        onLoadMore = viewModel::loadMore,
        onAction = viewModel::perform,
        onRemove = viewModel::remove,
        onPlay = { row ->
            val label = row.playerTitle(resources.getString(R.string.episode_number, row.item.episode))
            // Downloads carry no anime id; the watch-progress store keys on the item alone.
            onPlay(listOf(PlayableEpisode(row.item.id, label)), 0, label)
        },
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DownloadsScreen(
    state: DownloadsUiState,
    snackbar: SnackbarHostState,
    onGroup: (StatusGroup) -> Unit,
    onQuery: (String) -> Unit,
    onRefresh: () -> Unit,
    onLoadMore: () -> Unit,
    onAction: (DownloadRow, DownloadAction) -> Unit,
    onRemove: (DownloadRow) -> Unit,
    onPlay: (DownloadRow) -> Unit,
) {
    var confirmRemove by remember { mutableStateOf<DownloadRow?>(null) }
    Scaffold(
        topBar = { TopAppBar(title = { Text(stringResource(R.string.downloads)) }) },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding)) {
            OutlinedTextField(
                value = state.query,
                onValueChange = onQuery,
                placeholder = { Text(stringResource(R.string.search_hint)) },
                leadingIcon = { Icon(painterResource(AppIcons.Search), contentDescription = null) },
                singleLine = true,
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
                modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp).testTag(DownloadsTestTags.SEARCH),
            )
            LazyRow(
                contentPadding = PaddingValues(horizontal = 16.dp, vertical = 8.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                items(StatusGroup.entries, key = { it.name }) { group ->
                    val count = state.counts?.let { if (group == StatusGroup.All) it.values.sum() else group.count(it) }
                    FilterChip(
                        selected = state.group == group,
                        onClick = { onGroup(group) },
                        label = { Text(if (count == null) stringResource(group.label()) else "${stringResource(group.label())} $count") },
                        modifier = Modifier.testTag(DownloadsTestTags.chip(group)),
                    )
                }
            }
            if (state.stale) Text(
                stringResource(R.string.update_failed),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.error,
                modifier = Modifier.padding(horizontal = 16.dp),
            )
            val body = Modifier.fillMaxSize()
            val error = state.error
            when {
                state.items.isEmpty() && error != null -> ErrorState(error, onRetry = onRefresh, modifier = body)
                state.items.isEmpty() && state.loading -> LoadingState(body)
                state.items.isEmpty() -> Box(body, contentAlignment = Alignment.Center) {
                    Text(
                        stringResource(if (state.query.isBlank() && state.group == StatusGroup.All) R.string.downloads_empty else R.string.downloads_no_match),
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                else -> PullToRefreshBox(isRefreshing = state.refreshing, onRefresh = onRefresh, modifier = body) {
                    DownloadList(state, onLoadMore, onAction, onRemove = { confirmRemove = it }, onPlay)
                }
            }
        }
    }
    confirmRemove?.let { row ->
        AlertDialog(
            onDismissRequest = { confirmRemove = null },
            title = { Text(stringResource(R.string.remove_title)) },
            text = { Text(stringResource(R.string.remove_message, row.displayTitle())) },
            confirmButton = {
                TextButton(onClick = { confirmRemove = null; onRemove(row) }, modifier = Modifier.testTag(DownloadsTestTags.CONFIRM_REMOVE)) {
                    Text(stringResource(R.string.remove), color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = { TextButton(onClick = { confirmRemove = null }) { Text(stringResource(R.string.cancel)) } },
        )
    }
}

@Composable
private fun DownloadList(
    state: DownloadsUiState,
    onLoadMore: () -> Unit,
    onAction: (DownloadRow, DownloadAction) -> Unit,
    onRemove: (DownloadRow) -> Unit,
    onPlay: (DownloadRow) -> Unit,
) {
    val list = rememberLazyListState()
    val nearEnd by remember {
        derivedStateOf {
            val last = list.layoutInfo.visibleItemsInfo.lastOrNull()?.index ?: 0
            last >= list.layoutInfo.totalItemsCount - 5
        }
    }
    LaunchedEffect(nearEnd, state.endReached, state.items.size) {
        if (nearEnd && !state.endReached && state.appendError == null) onLoadMore()
    }
    LazyColumn(state = list, modifier = Modifier.fillMaxSize().testTag(DownloadsTestTags.LIST)) {
        items(state.items, key = { it.item.id }, contentType = { "item" }) { row ->
            DownloadRowItem(
                row = row,
                busy = row.item.id in state.busy,
                onAction = { onAction(row, it) },
                onRemove = { onRemove(row) },
                onPlay = { onPlay(row) },
            )
            HorizontalDivider(Modifier.padding(start = 16.dp))
        }
        if (!state.endReached) item(contentType = "footer") {
            Box(Modifier.fillMaxWidth().padding(16.dp), contentAlignment = Alignment.Center) {
                val error = state.appendError
                if (error != null) TextButton(onClick = onLoadMore) { Text(error.message()) }
                else CircularProgressIndicator()
            }
        }
    }
}

@Composable
private fun DownloadRowItem(
    row: DownloadRow,
    busy: Boolean,
    onAction: (DownloadAction) -> Unit,
    onRemove: () -> Unit,
    onPlay: () -> Unit,
) {
    val item = row.item
    val status = item.effectiveStatus
    val playable = item.isPlayable()
    Row(
        Modifier
            .fillMaxWidth()
            .then(if (playable) Modifier.clickable(onClick = onPlay) else Modifier)
            .padding(start = 16.dp, top = 12.dp, end = 4.dp, bottom = 12.dp)
            .testTag(DownloadsTestTags.row(item.id)),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        CoverImage(
            url = row.coverUrl,
            contentDescription = null,
            modifier = Modifier.size(width = 48.dp, height = 72.dp).clip(RoundedCornerShape(8.dp)),
        )
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(
                row.displayTitle() + (item.episode.takeIf { it.isNotEmpty() }?.let { " · " + stringResource(R.string.episode_number, it) } ?: ""),
                style = MaterialTheme.typography.titleSmall,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                listOf(
                    item.fansub_group.ifEmpty { stringResource(R.string.unknown_group) },
                    item.quality.ifEmpty { stringResource(R.string.unknown_quality) },
                ).joinToString(" · "),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            // Status on the left, actions on the right, so a row stays about as tall as its cover.
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Text(
                    statusLabel(status),
                    style = MaterialTheme.typography.labelMedium,
                    color = statusColor(status),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f).testTag(DownloadsTestTags.status(item.id)),
                )
                if (busy) CircularProgressIndicator(Modifier.padding(12.dp).size(20.dp), strokeWidth = 2.dp)
                if (playable) ActionIcon(AppIcons.Play, R.string.play, enabled = !busy, tag = DownloadsTestTags.PLAY, onClick = onPlay)
                if (item.canRetry()) ActionIcon(
                    AppIcons.Download,
                    if (status == STATUS_PENDING) R.string.download else R.string.retry_download,
                    enabled = !busy,
                    tag = DownloadsTestTags.DOWNLOAD,
                ) { onAction(DownloadAction.Download) }
                if (item.canPause()) ActionIcon(AppIcons.Pause, R.string.pause, enabled = !busy, tag = DownloadsTestTags.PAUSE) { onAction(DownloadAction.Pause) }
                if (item.canResume()) ActionIcon(AppIcons.Resume, R.string.resume, enabled = !busy, tag = DownloadsTestTags.RESUME) { onAction(DownloadAction.Resume) }
                ActionIcon(AppIcons.Delete, R.string.remove_record, enabled = !busy, tag = DownloadsTestTags.REMOVE, onClick = onRemove)
            }
            val showProgress = isDownloadingStatus(status) || ((status == STATUS_PAUSED || status == STATUS_QUEUED_RESUME) && item.progress > 0)
            if (showProgress) {
                val progress = item.progress.coerceIn(0, 100)
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                    Text(progressHint(row), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Text("$progress%", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                LinearProgressIndicator(progress = { progress / 100f }, modifier = Modifier.fillMaxWidth().padding(end = 12.dp))
            }
            if (item.task_error.isNotEmpty()) Text(
                item.task_error,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.error,
                maxLines = 3,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.padding(end = 12.dp),
            )
        }
    }
}

@Composable
private fun ActionIcon(icon: Int, label: Int, enabled: Boolean, tag: String, onClick: () -> Unit) {
    IconButton(onClick = onClick, enabled = enabled, modifier = Modifier.testTag(tag)) {
        Icon(painterResource(icon), contentDescription = stringResource(label))
    }
}

@Composable
private fun progressHint(row: DownloadRow): String {
    val item = row.item
    return when (item.effectiveStatus) {
        STATUS_PAUSED -> stringResource(if (item.restartsOnResume) R.string.resume_restarts else R.string.progress_kept)
        STATUS_QUEUED_RESUME -> stringResource(R.string.waiting_resume_slot)
        STATUS_RESUMING -> stringResource(R.string.confirming_resume)
        else -> if (item.download_speed > 0) formatSpeed(item.download_speed) else stringResource(R.string.waiting_transfer)
    }
}

@Composable
private fun statusLabel(status: String): String = when {
    status == STATUS_PENDING -> stringResource(R.string.group_pending)
    status == STATUS_QUEUED -> stringResource(R.string.status_queued)
    status == STATUS_QUEUED_RESUME -> stringResource(R.string.status_queued_resume)
    status == STATUS_PAUSED -> stringResource(R.string.group_paused)
    status == STATUS_SAVED -> stringResource(R.string.group_saved)
    status == STATUS_FAILED -> stringResource(R.string.group_failed)
    status == STATUS_WAITING_DOWNLOADER -> stringResource(R.string.group_waiting_downloader)
    status == STATUS_DOWNLOADING -> stringResource(R.string.group_downloading)
    status == STATUS_RESUMING -> stringResource(R.string.status_resuming)
    status.startsWith(SENT_TO_PREFIX) -> stringResource(R.string.status_sent_to, status.removePrefix(SENT_TO_PREFIX).trim())
    // Statuses this client does not know yet are shown as the server wrote them.
    else -> status
}

/** Same tones as the web status badge. */
@Composable
private fun statusColor(status: String): Color = when {
    status == STATUS_SAVED -> MaterialTheme.colorScheme.primary
    status == STATUS_FAILED -> MaterialTheme.colorScheme.error
    isDownloadingStatus(status) -> LocalStatusColors.current.active
    status == STATUS_PAUSED || status == STATUS_WAITING_DOWNLOADER -> LocalStatusColors.current.waiting
    else -> MaterialTheme.colorScheme.onSurfaceVariant
}

private fun StatusGroup.label(): Int = when (this) {
    StatusGroup.All -> R.string.group_all
    StatusGroup.Pending -> R.string.group_pending
    StatusGroup.Downloading -> R.string.group_downloading
    StatusGroup.Paused -> R.string.group_paused
    StatusGroup.Saved -> R.string.group_saved
    StatusGroup.Failed -> R.string.group_failed
    StatusGroup.WaitingDownloader -> R.string.group_waiting_downloader
}

private fun DownloadMessage.textRes(): Int = when (this) {
    is DownloadMessage.Done -> when (action) {
        DownloadAction.Download -> R.string.queued_download
        DownloadAction.Pause -> R.string.paused
        DownloadAction.Resume -> R.string.queued_resume
    }
    DownloadMessage.Unchanged -> R.string.already_in_state
    DownloadMessage.Removed -> R.string.removed
    is DownloadMessage.Failed -> error("Failures carry their own text")
}

internal fun DownloadRow.displayTitle(): String = item.anime_title.ifEmpty { item.title }

internal fun DownloadRow.playerTitle(episodeLabel: String): String =
    if (item.episode.isEmpty()) displayTitle() else "${displayTitle()} $episodeLabel"

/** Mirrors the web formatter: binary units, one decimal above bytes. */
internal fun formatSpeed(bytesPerSecond: Long): String {
    var value = bytesPerSecond.coerceAtLeast(0).toDouble()
    val units = listOf("B/s", "KB/s", "MB/s", "GB/s")
    var unit = 0
    while (value >= 1024 && unit < units.lastIndex) {
        value /= 1024
        unit++
    }
    return if (unit == 0) "${value.toLong()} ${units[0]}" else "%.1f %s".format(java.util.Locale.ROOT, value, units[unit])
}

object DownloadsTestTags {
    const val LIST = "downloads_list"
    const val SEARCH = "downloads_search"
    const val DOWNLOAD = "download_action"
    const val PAUSE = "pause_action"
    const val RESUME = "resume_action"
    const val REMOVE = "remove_action"
    const val PLAY = "play_action"
    const val CONFIRM_REMOVE = "confirm_remove"
    fun row(id: Long) = "download_row_$id"
    fun status(id: Long) = "download_status_$id"
    fun chip(group: StatusGroup) = "status_chip_${group.name}"
}
