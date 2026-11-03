package com.autoanime.android.feature.discover

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
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
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.autoanime.android.core.designsystem.AppIcons
import com.autoanime.android.core.designsystem.ErrorState
import com.autoanime.android.core.designsystem.LoadingState
import com.autoanime.android.core.designsystem.LocalStatusColors
import com.autoanime.android.core.designsystem.message
import com.autoanime.android.core.designsystem.R as DesignR

@Composable
fun SubjectRoute(subjectId: Long, subjectName: String, onBack: (() -> Unit)?) {
    val viewModel = hiltViewModel<SubjectViewModel, SubjectViewModel.Factory>(
        key = "subject-$subjectId",
        creationCallback = { it.create(subjectId, subjectName) },
    )
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }
    val resources = LocalResources.current
    LaunchedEffect(viewModel) {
        viewModel.messages.collect { message ->
            snackbar.showSnackbar(
                when (message) {
                    is SubjectMessage.Subscribed -> message.added?.let { resources.getString(R.string.subscribed_synced, it) }
                        ?: resources.getString(R.string.subscribed)
                    is SubjectMessage.Failed -> message.error.message(resources)
                },
            )
        }
    }
    SubjectScreen(subjectName, state, snackbar, onBack, viewModel::load, viewModel::defaultName, viewModel::subscribe)
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SubjectScreen(
    title: String,
    state: SubjectUiState,
    snackbar: SnackbarHostState,
    onBack: (() -> Unit)?,
    onRetry: () -> Unit,
    defaultName: (ResourceVersion) -> String,
    onSubscribe: (ResourceVersion, name: String, autoDownload: Boolean) -> Unit,
) {
    var confirm by remember { mutableStateOf<ResourceVersion?>(null) }
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(title, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                navigationIcon = {
                    if (onBack != null) IconButton(onClick = onBack) {
                        Icon(painterResource(AppIcons.Back), contentDescription = stringResource(DesignR.string.back))
                    }
                },
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { padding ->
        val body = Modifier.fillMaxSize().padding(padding)
        val error = state.error
        when {
            error != null && state.versions.isEmpty() -> ErrorState(error, onRetry = onRetry, modifier = body)
            state.loading && state.versions.isEmpty() -> LoadingState(body)
            else -> LazyColumn(body) {
                items(state.warnings) {
                    Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp))
                }
                if (state.versions.isEmpty()) item {
                    Text(
                        stringResource(R.string.no_resources),
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(16.dp),
                    )
                }
                items(state.versions, key = { it.key }, contentType = { "version" }) { version ->
                    VersionRow(version, subscribing = version.key in state.subscribing, onSubscribe = { confirm = version })
                    HorizontalDivider(Modifier.padding(start = 16.dp))
                }
                if (state.unusable > 0) item {
                    Text(
                        stringResource(R.string.unusable_resources, state.unusable),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(16.dp),
                    )
                }
            }
        }
    }
    confirm?.let { version ->
        SubscribeDialog(
            version = version,
            initialName = defaultName(version),
            onDismiss = { confirm = null },
            onConfirm = { name, auto -> confirm = null; onSubscribe(version, name, auto) },
        )
    }
}

@Composable
private fun VersionRow(version: ResourceVersion, subscribing: Boolean, onSubscribe: () -> Unit) {
    Column(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(versionLabel(version), style = MaterialTheme.typography.titleSmall)
        Text(
            stringResource(R.string.release_summary, version.releases.size, episodeRange(version)),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Text(
            version.releases.maxByOrNull { it.published }?.title.orEmpty(),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
        )
        if (version.needsReview) Text(
            stringResource(R.string.needs_review),
            style = MaterialTheme.typography.labelMedium,
            color = LocalStatusColors.current.waiting,
        )
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End, verticalAlignment = Alignment.CenterVertically) {
            when {
                version.subscribedFeedId != null -> Text(stringResource(R.string.already_subscribed), color = MaterialTheme.colorScheme.primary)
                subscribing -> CircularProgressIndicator(Modifier.size(24.dp), strokeWidth = 2.dp)
                else -> OutlinedButton(onClick = onSubscribe, modifier = Modifier.testTag(DiscoverTestTags.SUBSCRIBE)) {
                    Text(stringResource(R.string.subscribe_version))
                }
            }
        }
    }
}

/** Shows the season rule plainly, since subscriptions created here are strict about it. */
@Composable
private fun SubscribeDialog(version: ResourceVersion, initialName: String, onDismiss: () -> Unit, onConfirm: (String, Boolean) -> Unit) {
    var name by remember { mutableStateOf(initialName) }
    var auto by remember { mutableStateOf(true) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.subscribe_version)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                OutlinedTextField(value = name, onValueChange = { name = it }, label = { Text(stringResource(R.string.feed_name)) }, singleLine = true)
                Text(
                    if (version.season > 0) stringResource(R.string.season_rule_specific, version.season)
                    else stringResource(R.string.season_rule_unmarked),
                    style = MaterialTheme.typography.bodySmall,
                )
                Row(
                    Modifier.fillMaxWidth().toggleable(auto, role = Role.Switch, onValueChange = { auto = it }),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(stringResource(R.string.auto_download), Modifier.weight(1f))
                    Switch(checked = auto, onCheckedChange = null)
                }
            }
        },
        confirmButton = {
            Button(onClick = { onConfirm(name, auto) }, modifier = Modifier.testTag(DiscoverTestTags.CONFIRM_SUBSCRIBE)) {
                Text(stringResource(R.string.subscribe))
            }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) } },
    )
}

@Composable
private fun versionLabel(version: ResourceVersion): String = listOf(
    version.group.ifBlank { stringResource(R.string.unknown_group) },
    version.quality.ifBlank { stringResource(R.string.unknown_quality) },
    version.language,
    if (version.season > 0) stringResource(R.string.season_label, version.season) else "",
).filter(String::isNotBlank).joinToString(" · ")

/** "1–7" for numeric episodes, otherwise the distinct labels; empty when none are known. */
internal fun episodeRange(version: ResourceVersion): String {
    val labels = version.releases.map { it.episode.trim() }.filter(String::isNotEmpty).distinct()
    val numbers = labels.mapNotNull(String::toIntOrNull)
    return when {
        labels.isEmpty() -> "—"
        numbers.size == labels.size && numbers.size > 1 -> "${numbers.min()}–${numbers.max()}"
        else -> labels.take(5).joinToString(", ") + if (labels.size > 5) "…" else ""
    }
}
