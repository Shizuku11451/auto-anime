package com.autoanime.android.feature.library

import android.widget.Toast
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringArrayResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.autoanime.android.core.designsystem.AppIcons
import com.autoanime.android.core.designsystem.ErrorState
import com.autoanime.android.core.designsystem.LoadingState
import com.autoanime.android.core.designsystem.message
import com.autoanime.android.core.designsystem.R as DesignR

@Composable
fun AnimeEditRoute(animeId: Long, onDone: () -> Unit) {
    val viewModel = hiltViewModel<AnimeEditViewModel, AnimeEditViewModel.Factory>(
        key = "anime-edit-$animeId",
        creationCallback = { it.create(animeId) },
    )
    val state by viewModel.state.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val resources = LocalResources.current
    state.done?.let { outcome ->
        LaunchedEffect(outcome) {
            val text = buildString {
                append(outcome.matched?.let { resources.getString(R.string.rules_applied, it) } ?: resources.getString(R.string.anime_saved))
                if (outcome.storageWarning.isNotBlank()) append("\n").append(outcome.storageWarning)
            }
            Toast.makeText(context, text, Toast.LENGTH_LONG).show()
            onDone()
        }
    }
    AnimeEditScreen(state, onDone, viewModel::update, viewModel::save, viewModel::saveAndApply, viewModel::load)
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AnimeEditScreen(
    state: AnimeEditUiState,
    onBack: () -> Unit,
    onChange: ((AnimeForm) -> AnimeForm) -> Unit,
    onSave: () -> Unit,
    onSaveAndApply: () -> Unit,
    onRetry: () -> Unit,
) {
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.edit_anime)) },
                navigationIcon = { BackButton(onBack) },
                actions = {
                    if (state.saving) CircularProgressIndicator(Modifier.padding(12.dp).size(24.dp), strokeWidth = 2.dp)
                    else TextButton(onClick = onSave, enabled = !state.loading && state.loadError == null, modifier = Modifier.testTag(LibraryTestTags.SAVE_ANIME)) {
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
                val form = state.form
                val wide = Modifier.fillMaxWidth().widthIn(max = 640.dp)
                OutlinedTextField(
                    value = form.title,
                    onValueChange = { v -> onChange { it.copy(title = v) } },
                    label = { Text(stringResource(R.string.anime_title)) },
                    singleLine = true,
                    isError = state.titleMissing,
                    supportingText = if (state.titleMissing) ({ Text(stringResource(R.string.title_required)) }) else null,
                    modifier = wide,
                )
                OutlinedTextField(
                    value = form.searchName,
                    onValueChange = { v -> onChange { it.copy(searchName = v) } },
                    label = { Text(stringResource(R.string.search_name)) },
                    singleLine = true,
                    modifier = wide,
                )
                OutlinedTextField(
                    value = form.season,
                    onValueChange = { v -> onChange { it.copy(season = v.filter(Char::isDigit).take(3)) } },
                    label = { Text(stringResource(R.string.default_season)) },
                    singleLine = true,
                    isError = state.seasonInvalid,
                    supportingText = if (state.seasonInvalid) ({ Text(stringResource(R.string.season_range)) }) else null,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = Modifier.widthIn(max = 200.dp),
                )
                Text(stringResource(R.string.update_day), style = MaterialTheme.typography.labelLarge)
                WeekdayPicker(form.updateWeekday) { day -> onChange { it.copy(updateWeekday = day) } }
                OutlinedTextField(
                    value = form.include,
                    onValueChange = { v -> onChange { it.copy(include = v) } },
                    label = { Text(stringResource(R.string.include_rules)) },
                    isError = state.includeMissing,
                    supportingText = if (state.includeMissing) ({ Text(stringResource(R.string.include_required)) }) else null,
                    modifier = wide,
                )
                OutlinedTextField(
                    value = form.exclude,
                    onValueChange = { v -> onChange { it.copy(exclude = v) } },
                    label = { Text(stringResource(R.string.exclude_rules)) },
                    modifier = wide,
                )
                OutlinedButton(onClick = onSaveAndApply, enabled = !state.saving) { Text(stringResource(R.string.save_and_apply)) }
                Text(
                    stringResource(R.string.apply_rules_hint),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                state.saveError?.let { Text(it.message(), color = MaterialTheme.colorScheme.error) }
            }
        }
    }
}

/** Automatic (Bangumi), then Monday to Sunday. */
@Composable
private fun WeekdayPicker(selected: Int, onSelect: (Int) -> Unit) {
    val weekdays = stringArrayResource(R.array.weekdays)
    androidx.compose.foundation.layout.FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        FilterChip(selected = selected == 0, onClick = { onSelect(0) }, label = { Text(stringResource(R.string.weekday_auto)) })
        weekdays.forEachIndexed { index, label ->
            FilterChip(selected = selected == index + 1, onClick = { onSelect(index + 1) }, label = { Text(label) })
        }
    }
}

@Composable
fun BindRoute(animeId: Long, onDone: () -> Unit) {
    val viewModel = hiltViewModel<BindViewModel, BindViewModel.Factory>(
        key = "anime-bind-$animeId",
        creationCallback = { it.create(animeId) },
    )
    val state by viewModel.state.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val resources = LocalResources.current
    if (state.done) LaunchedEffect(Unit) {
        Toast.makeText(context, resources.getString(R.string.binding_saved), Toast.LENGTH_SHORT).show()
        onDone()
    }
    BindScreen(state, onDone, viewModel::setQuery, viewModel::search, viewModel::bind)
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BindScreen(
    state: BindUiState,
    onBack: () -> Unit,
    onQuery: (String) -> Unit,
    onSearch: () -> Unit,
    onBind: (Long) -> Unit,
) {
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.bind_metadata)) },
                navigationIcon = { BackButton(onBack) },
                actions = {
                    if (state.bound) TextButton(onClick = { onBind(0) }, enabled = state.binding == null) {
                        Text(stringResource(R.string.unbind), color = MaterialTheme.colorScheme.error)
                    }
                },
            )
        },
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding)) {
            OutlinedTextField(
                value = state.query,
                onValueChange = onQuery,
                label = { Text(stringResource(R.string.bangumi_keyword)) },
                singleLine = true,
                isError = state.queryTooShort,
                supportingText = if (state.queryTooShort) ({ Text(stringResource(R.string.keyword_too_short)) }) else null,
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
                keyboardActions = KeyboardActions(onSearch = { onSearch() }),
                trailingIcon = {
                    IconButton(onClick = onSearch) { Icon(painterResource(AppIcons.Search), contentDescription = stringResource(R.string.search)) }
                },
                modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp),
            )
            if (state.searching) CircularProgressIndicator(Modifier.padding(16.dp))
            state.error?.let { Text(it.message(), color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(16.dp)) }
            if (state.searched && state.results.isEmpty() && !state.searching) Text(
                stringResource(R.string.no_candidates),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(16.dp),
            )
            LazyColumn(Modifier.fillMaxSize()) {
                items(state.results, key = { it.id }) { candidate ->
                    ListItem(
                        headlineContent = { Text(candidate.name_cn.ifBlank { candidate.name }) },
                        supportingContent = { Text(listOf(candidate.name, "#${candidate.id}").filter(String::isNotBlank).joinToString(" · ")) },
                        trailingContent = {
                            if (state.binding == candidate.id) CircularProgressIndicator(Modifier.size(24.dp), strokeWidth = 2.dp)
                        },
                        modifier = Modifier.clickable(enabled = state.binding == null) { onBind(candidate.id) },
                    )
                    HorizontalDivider()
                }
            }
        }
    }
}

@Composable
internal fun BackButton(onBack: () -> Unit) {
    IconButton(onClick = onBack) {
        Icon(painterResource(AppIcons.Back), contentDescription = stringResource(DesignR.string.back))
    }
}
