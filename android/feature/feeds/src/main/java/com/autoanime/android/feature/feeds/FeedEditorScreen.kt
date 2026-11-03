package com.autoanime.android.feature.feeds

import android.widget.Toast
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.autoanime.android.core.designsystem.AppIcons
import com.autoanime.android.core.designsystem.ErrorState
import com.autoanime.android.core.designsystem.LoadingState
import com.autoanime.android.core.designsystem.message
import com.autoanime.android.core.designsystem.R as DesignR

@Composable
fun FeedEditorRoute(feedId: Long?, onDone: () -> Unit) {
    val viewModel = hiltViewModel<FeedEditorViewModel, FeedEditorViewModel.Factory>(
        key = "feed-editor-${feedId ?: "new"}",
        creationCallback = { it.create(feedId) },
    )
    val state by viewModel.state.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val resources = LocalResources.current
    state.saved?.let { outcome ->
        LaunchedEffect(outcome) {
            val text = buildString {
                append(resources.getString(if (outcome.created) R.string.feed_created else R.string.feed_saved))
                outcome.added?.let { append(" · ").append(resources.getString(R.string.synced, it)) }
                if (outcome.storageWarning.isNotBlank()) append("\n").append(outcome.storageWarning)
            }
            Toast.makeText(context, text, Toast.LENGTH_LONG).show()
            onDone()
        }
    }
    FeedEditorScreen(state, onBack = onDone, viewModel::update, viewModel::preview, viewModel::save, viewModel::retryLoad)
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FeedEditorScreen(
    state: FeedEditorUiState,
    onBack: () -> Unit,
    onChange: ((FeedForm) -> FeedForm) -> Unit,
    onPreview: () -> Unit,
    onSave: () -> Unit,
    onRetry: () -> Unit,
) {
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(if (state.isNew) R.string.add_feed else R.string.edit_feed)) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(painterResource(AppIcons.Back), contentDescription = stringResource(DesignR.string.back))
                    }
                },
                actions = {
                    if (state.saving) CircularProgressIndicator(Modifier.padding(12.dp).size(24.dp), strokeWidth = 2.dp)
                    else TextButton(onClick = onSave, enabled = !state.loading && state.loadError == null, modifier = Modifier.testTag(FeedsTestTags.SAVE)) {
                        Text(stringResource(R.string.save))
                    }
                },
            )
        },
    ) { padding ->
        val body = Modifier.fillMaxSize().padding(padding)
        val loadError = state.loadError
        when {
            loadError != null -> ErrorState(loadError, onRetry = onRetry, modifier = body)
            state.loading -> LoadingState(body)
            else -> Column(
                body.imePadding().verticalScroll(rememberScrollState()).padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                FeedFields(state, onChange)
                PreviewSection(state, onPreview)
                state.saveError?.let { Text(it.message(), color = MaterialTheme.colorScheme.error) }
            }
        }
    }
}

@Composable
private fun FeedFields(state: FeedEditorUiState, onChange: ((FeedForm) -> FeedForm) -> Unit) {
    val form = state.form
    val wide = Modifier.fillMaxWidth().widthIn(max = 640.dp)
    OutlinedTextField(
        value = form.name,
        onValueChange = { v -> onChange { it.copy(name = v) } },
        label = { Text(stringResource(R.string.feed_name)) },
        singleLine = true,
        isError = state.problem == FormProblem.Name,
        supportingText = if (state.problem == FormProblem.Name) ({ Text(stringResource(R.string.name_required)) }) else null,
        modifier = wide.testTag(FeedsTestTags.NAME),
    )
    if (state.isNew) OutlinedTextField(
        value = form.url,
        onValueChange = { v -> onChange { it.copy(url = v) } },
        label = { Text(stringResource(R.string.rss_url)) },
        singleLine = true,
        isError = state.problem == FormProblem.Url,
        supportingText = if (state.problem == FormProblem.Url) ({ Text(stringResource(R.string.url_invalid)) }) else null,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
        modifier = wide.testTag(FeedsTestTags.URL),
    ) else Text(
        stringResource(if (state.aggregate) R.string.aggregate_feed else R.string.url_fixed),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    OutlinedTextField(
        value = form.targetTitle,
        onValueChange = { v -> onChange { it.copy(targetTitle = v) } },
        label = { Text(stringResource(R.string.target_title)) },
        singleLine = true,
        modifier = wide,
    )
    OutlinedTextField(
        value = form.include,
        onValueChange = { v -> onChange { it.copy(include = v) } },
        label = { Text(stringResource(R.string.include_words)) },
        singleLine = true,
        modifier = wide,
    )
    OutlinedTextField(
        value = form.exclude,
        onValueChange = { v -> onChange { it.copy(exclude = v) } },
        label = { Text(stringResource(R.string.exclude_words)) },
        singleLine = true,
        modifier = wide,
    )
    Text(stringResource(R.string.season_filter), style = MaterialTheme.typography.labelLarge)
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        listOf(SeasonMode.Any to R.string.season_any, SeasonMode.Unmarked to R.string.season_unmarked, SeasonMode.Specific to R.string.season_specific)
            .forEach { (mode, label) ->
                FilterChip(
                    selected = form.seasonMode == mode,
                    onClick = { onChange { it.copy(seasonMode = mode) } },
                    label = { Text(stringResource(label)) },
                )
            }
    }
    if (form.seasonMode == SeasonMode.Specific) {
        OutlinedTextField(
            value = form.season,
            onValueChange = { v -> onChange { it.copy(season = v.filter(Char::isDigit).take(3)) } },
            label = { Text(stringResource(R.string.season_number)) },
            singleLine = true,
            isError = state.problem == FormProblem.Season,
            supportingText = if (state.problem == FormProblem.Season) ({ Text(stringResource(R.string.season_invalid)) }) else null,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
            modifier = Modifier.widthIn(max = 200.dp),
        )
        SwitchRow(R.string.allow_unknown_season, form.allowUnknownSeason) { v -> onChange { it.copy(allowUnknownSeason = v) } }
    }
    SwitchRow(R.string.auto_download, form.autoDownload) { v -> onChange { it.copy(autoDownload = v) } }
}

@Composable
private fun SwitchRow(label: Int, checked: Boolean, onChange: (Boolean) -> Unit) {
    Row(
        Modifier.fillMaxWidth().widthIn(max = 640.dp).toggleable(checked, role = Role.Switch, onValueChange = onChange),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(stringResource(label), Modifier.weight(1f), style = MaterialTheme.typography.bodyLarge)
        Switch(checked = checked, onCheckedChange = null)
    }
}

@Composable
private fun PreviewSection(state: FeedEditorUiState, onPreview: () -> Unit) {
    OutlinedButton(onClick = onPreview, enabled = !state.previewing, modifier = Modifier.testTag(FeedsTestTags.PREVIEW)) {
        if (state.previewing) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
        else Text(stringResource(R.string.preview))
    }
    state.previewError?.let { Text(it.message(), color = MaterialTheme.colorScheme.error) }
    val preview = state.preview ?: return
    Text(stringResource(R.string.preview_count, preview.count, preview.total), style = MaterialTheme.typography.titleSmall)
    if (preview.warning.isNotBlank()) Text(preview.warning, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
    preview.items.take(PREVIEW_ROWS).forEach {
        Text(it.title, style = MaterialTheme.typography.bodySmall, maxLines = 2, overflow = TextOverflow.Ellipsis)
    }
    if (preview.items.size > PREVIEW_ROWS) Text(
        stringResource(R.string.preview_more, preview.items.size - PREVIEW_ROWS),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

private const val PREVIEW_ROWS = 20
