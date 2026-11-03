package com.autoanime.android.feature.settings

import androidx.compose.foundation.layout.Column
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
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.autoanime.android.core.designsystem.AppIcons
import com.autoanime.android.core.designsystem.ErrorState
import com.autoanime.android.core.designsystem.LoadingState
import com.autoanime.android.core.designsystem.message
import com.autoanime.android.core.model.MAX_CONCURRENT_DOWNLOADS
import com.autoanime.android.core.model.MAX_POLL_MINUTES
import com.autoanime.android.core.model.MIN_POLL_MINUTES
import com.autoanime.android.core.designsystem.R as DesignR

@Composable
fun ServerSettingsRoute(onBack: () -> Unit, viewModel: ServerSettingsViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    ServerSettingsScreen(state, onBack, viewModel::update, viewModel::save, viewModel::load)
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ServerSettingsScreen(
    state: ServerSettingsUiState,
    onBack: () -> Unit,
    onChange: ((ServerForm) -> ServerForm) -> Unit,
    onSave: () -> Unit,
    onRetry: () -> Unit,
) {
    var confirmCleanup by remember { mutableStateOf(false) }
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.server_settings)) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(painterResource(AppIcons.Back), contentDescription = stringResource(DesignR.string.back))
                    }
                },
                actions = {
                    if (state.saving) CircularProgressIndicator(Modifier.padding(12.dp).size(24.dp), strokeWidth = 2.dp)
                    else TextButton(onClick = onSave, enabled = state.changed, modifier = Modifier.testTag(SettingsTestTags.SAVE_SERVER)) {
                        Text(stringResource(R.string.save))
                    }
                },
            )
        },
    ) { padding ->
        val body = Modifier.fillMaxSize().padding(padding)
        val loadError = state.loadError
        val stored = state.stored
        when {
            loadError != null && stored == null -> ErrorState(loadError, onRetry = onRetry, modifier = body)
            stored == null -> LoadingState(body)
            else -> Column(body.imePadding().verticalScroll(rememberScrollState()).padding(vertical = 8.dp)) {
                val form = state.form
                val field = Modifier.fillMaxWidth().widthIn(max = 640.dp).padding(horizontal = 16.dp, vertical = 4.dp)
                OutlinedTextField(
                    value = form.pollMinutes,
                    onValueChange = { v -> onChange { it.copy(pollMinutes = v.filter(Char::isDigit).take(4)) } },
                    label = { Text(stringResource(R.string.poll_minutes)) },
                    supportingText = { Text(stringResource(R.string.poll_minutes_range, MIN_POLL_MINUTES, MAX_POLL_MINUTES)) },
                    isError = state.invalid == ServerField.PollMinutes,
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = field,
                )
                OutlinedTextField(
                    value = form.maxConcurrent,
                    onValueChange = { v -> onChange { it.copy(maxConcurrent = v.filter(Char::isDigit).take(2)) } },
                    label = { Text(stringResource(R.string.max_concurrent)) },
                    supportingText = { Text(stringResource(R.string.max_concurrent_range, MAX_CONCURRENT_DOWNLOADS)) },
                    isError = state.invalid == ServerField.MaxConcurrent,
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = field,
                )
                OutlinedTextField(
                    value = form.libraryDir,
                    onValueChange = { v -> onChange { it.copy(libraryDir = v) } },
                    label = { Text(stringResource(R.string.library_dir)) },
                    supportingText = { Text(stringResource(R.string.library_dir_hint, stored.download_dir)) },
                    singleLine = true,
                    modifier = field,
                )
                SwitchItem(R.string.keep_source_files, R.string.keep_source_files_hint, form.keepSourceFiles) { v ->
                    onChange { it.copy(keepSourceFiles = v) }
                }
                SwitchItem(R.string.auto_cleanup, R.string.auto_cleanup_hint, form.autoCleanup) { v ->
                    if (v && !stored.auto_cleanup_empty_anime) confirmCleanup = true else onChange { it.copy(autoCleanup = v) }
                }
                HorizontalDivider(Modifier.padding(vertical = 8.dp))
                ListItem(
                    headlineContent = { Text(stringResource(R.string.download_mode)) },
                    supportingContent = { Text(downloadModeLabel(stored.download_mode)) },
                )
                ListItem(
                    headlineContent = { Text(stringResource(R.string.proxy)) },
                    supportingContent = { Text(stored.proxy_url.ifBlank { stringResource(R.string.proxy_direct) }) },
                )
                Text(
                    stringResource(R.string.web_only_settings),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(horizontal = 16.dp),
                )
                state.saveError?.let { Text(it.message(), color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(16.dp)) }
                state.saved?.let { saved ->
                    val notes = listOfNotNull(
                        stringResource(R.string.server_settings_saved),
                        saved.cleaned_anime.takeIf { it > 0 }?.let { stringResource(R.string.cleaned_anime, it) },
                        saved.cleanup_warning.takeIf(String::isNotBlank),
                        saved.library_warning.takeIf(String::isNotBlank),
                    )
                    Text(notes.joinToString("\n"), color = MaterialTheme.colorScheme.primary, modifier = Modifier.padding(16.dp))
                }
            }
        }
    }
    if (confirmCleanup) AlertDialog(
        onDismissRequest = { confirmCleanup = false },
        title = { Text(stringResource(R.string.auto_cleanup)) },
        text = { Text(stringResource(R.string.auto_cleanup_confirm)) },
        confirmButton = {
            TextButton(onClick = { confirmCleanup = false; onChange { it.copy(autoCleanup = true) } }) { Text(stringResource(R.string.enable)) }
        },
        dismissButton = { TextButton(onClick = { confirmCleanup = false }) { Text(stringResource(R.string.cancel)) } },
    )
}

@Composable
private fun SwitchItem(title: Int, summary: Int, checked: Boolean, onChange: (Boolean) -> Unit) {
    ListItem(
        headlineContent = { Text(stringResource(title)) },
        supportingContent = { Text(stringResource(summary)) },
        trailingContent = { Switch(checked = checked, onCheckedChange = null) },
        modifier = Modifier.toggleable(checked, role = Role.Switch, onValueChange = onChange),
    )
}

@Composable
private fun downloadModeLabel(mode: String): String = when (mode) {
    "native" -> stringResource(R.string.mode_native)
    "direct" -> stringResource(R.string.mode_direct)
    "aria2" -> "Aria2"
    "qbittorrent" -> "qBittorrent"
    "transmission" -> "Transmission"
    else -> mode
}
