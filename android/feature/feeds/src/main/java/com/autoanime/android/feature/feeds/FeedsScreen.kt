package com.autoanime.android.feature.feeds

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
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.LifecycleStartEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.autoanime.android.core.designsystem.AppIcons
import com.autoanime.android.core.designsystem.ErrorState
import com.autoanime.android.core.designsystem.LoadingState
import com.autoanime.android.core.designsystem.message
import com.autoanime.android.core.model.Feed
import com.autoanime.android.core.model.displayHost

@Composable
fun FeedsRoute(onEdit: (feedId: Long?) -> Unit, viewModel: FeedsViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    // Re-read on every return, so edits made in the editor show up.
    LifecycleStartEffect(viewModel) {
        viewModel.load()
        onStopOrDispose { }
    }
    val snackbar = remember { SnackbarHostState() }
    val resources = LocalResources.current
    LaunchedEffect(viewModel) {
        viewModel.messages.collect { message ->
            snackbar.showSnackbar(
                when (message) {
                    is FeedMessage.Synced -> resources.getString(R.string.synced, message.added)
                    is FeedMessage.Toggled -> resources.getString(if (message.enabled) R.string.feed_enabled else R.string.feed_paused)
                    FeedMessage.Deleted -> resources.getString(R.string.feed_deleted)
                    is FeedMessage.Failed -> message.error.message(resources)
                },
            )
        }
    }
    FeedsScreen(state, snackbar, onRefresh = viewModel::load, onEdit, viewModel::toggle, viewModel::sync, viewModel::delete)
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FeedsScreen(
    state: FeedsUiState,
    snackbar: SnackbarHostState,
    onRefresh: () -> Unit,
    onEdit: (feedId: Long?) -> Unit,
    onToggle: (Feed) -> Unit,
    onSync: (Feed) -> Unit,
    onDelete: (Feed) -> Unit,
) {
    var confirmDelete by remember { mutableStateOf<Feed?>(null) }
    Scaffold(
        topBar = { TopAppBar(title = { Text(stringResource(R.string.feeds)) }) },
        snackbarHost = { SnackbarHost(snackbar) },
        floatingActionButton = {
            FloatingActionButton(onClick = { onEdit(null) }, modifier = Modifier.testTag(FeedsTestTags.ADD)) {
                Icon(painterResource(AppIcons.Add), contentDescription = stringResource(R.string.add_feed))
            }
        },
    ) { padding ->
        val body = Modifier.fillMaxSize().padding(padding)
        val error = state.error
        when {
            !state.loaded && error != null -> ErrorState(error, onRetry = onRefresh, modifier = body)
            !state.loaded -> LoadingState(body)
            state.feeds.isEmpty() -> Box(body, contentAlignment = Alignment.Center) {
                Text(stringResource(R.string.feeds_empty), color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            else -> PullToRefreshBox(isRefreshing = state.loading, onRefresh = onRefresh, modifier = body) {
                LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(bottom = 88.dp)) {
                    items(state.feeds, key = { it.id }, contentType = { "feed" }) { feed ->
                        FeedRow(
                            feed = feed,
                            busy = feed.id in state.busy,
                            onEdit = { onEdit(feed.id) },
                            onToggle = { onToggle(feed) },
                            onSync = { onSync(feed) },
                            onDelete = { confirmDelete = feed },
                        )
                        HorizontalDivider(Modifier.padding(start = 16.dp))
                    }
                }
            }
        }
    }
    confirmDelete?.let { feed ->
        AlertDialog(
            onDismissRequest = { confirmDelete = null },
            title = { Text(stringResource(R.string.delete_feed)) },
            text = { Text(stringResource(R.string.delete_feed_message, feed.name)) },
            confirmButton = {
                TextButton(onClick = { confirmDelete = null; onDelete(feed) }, modifier = Modifier.testTag(FeedsTestTags.CONFIRM_DELETE)) {
                    Text(stringResource(R.string.delete), color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = { TextButton(onClick = { confirmDelete = null }) { Text(stringResource(R.string.cancel)) } },
        )
    }
}

@Composable
private fun FeedRow(
    feed: Feed,
    busy: Boolean,
    onEdit: () -> Unit,
    onToggle: () -> Unit,
    onSync: () -> Unit,
    onDelete: () -> Unit,
) {
    Column(
        Modifier
            .fillMaxWidth()
            .clickable(onClick = onEdit)
            .padding(start = 16.dp, top = 12.dp, end = 8.dp, bottom = 4.dp)
            .testTag(FeedsTestTags.row(feed.id)),
        verticalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                Text(feed.name, style = MaterialTheme.typography.titleSmall, maxLines = 2, overflow = TextOverflow.Ellipsis)
                if (feed.target_title.isNotBlank() && feed.target_title != feed.name) Text(
                    feed.target_title,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            Switch(
                checked = feed.enabled,
                onCheckedChange = { onToggle() },
                enabled = !busy,
                modifier = Modifier.testTag(FeedsTestTags.TOGGLE),
            )
        }
        Text(
            feed.displayHost() ?: stringResource(R.string.aggregate_feed),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        Text(
            when {
                !feed.enabled -> stringResource(R.string.paused)
                feed.last_sync.isBlank() -> stringResource(R.string.never_synced)
                else -> stringResource(R.string.last_synced, formatTimestamp(feed.last_sync))
            },
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        if (feed.completion_note.isNotBlank()) Text(
            feed.completion_note,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.primary,
        )
        if (feed.last_error.isNotBlank()) Text(
            feed.last_error,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.error,
            maxLines = 3,
            overflow = TextOverflow.Ellipsis,
        )
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End, verticalAlignment = Alignment.CenterVertically) {
            if (busy) CircularProgressIndicator(Modifier.padding(12.dp).size(20.dp), strokeWidth = 2.dp)
            IconButton(onClick = onSync, enabled = !busy && feed.enabled, modifier = Modifier.testTag(FeedsTestTags.SYNC)) {
                Icon(painterResource(AppIcons.Refresh), contentDescription = stringResource(R.string.sync_now))
            }
            IconButton(onClick = onEdit, enabled = !busy) {
                Icon(painterResource(AppIcons.Edit), contentDescription = stringResource(R.string.edit))
            }
            IconButton(onClick = onDelete, enabled = !busy, modifier = Modifier.testTag(FeedsTestTags.DELETE)) {
                Icon(painterResource(AppIcons.Delete), contentDescription = stringResource(R.string.delete_feed))
            }
        }
    }
}

@Composable
fun FeedPlaceholder() {
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Text(stringResource(R.string.select_feed), color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

/** Server timestamps are RFC 3339; shown as local month, day and time, or as-is if unparsable. */
internal fun formatTimestamp(raw: String): String = runCatching {
    java.time.OffsetDateTime.parse(raw)
        .atZoneSameInstant(java.time.ZoneId.systemDefault())
        .format(java.time.format.DateTimeFormatter.ofPattern("MM-dd HH:mm"))
}.getOrDefault(raw)

object FeedsTestTags {
    const val ADD = "add_feed"
    const val TOGGLE = "feed_toggle"
    const val SYNC = "feed_sync"
    const val DELETE = "feed_delete"
    const val CONFIRM_DELETE = "confirm_delete_feed"
    const val NAME = "feed_name"
    const val URL = "feed_url"
    const val SAVE = "feed_save"
    const val PREVIEW = "feed_preview"
    fun row(id: Long) = "feed_row_$id"
}
