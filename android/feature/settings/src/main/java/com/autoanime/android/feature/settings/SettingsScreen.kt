package com.autoanime.android.feature.settings

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.foundation.clickable
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import com.autoanime.android.core.data.AppPreferences
import com.autoanime.android.core.data.SessionRepository
import com.autoanime.android.core.data.SessionState
import com.autoanime.android.core.designsystem.AppIcons
import com.autoanime.android.core.designsystem.supportsDynamicColor
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import javax.inject.Inject

data class SettingsUiState(val server: String = "", val account: String = "", val dynamicColor: Boolean = false)

@HiltViewModel
class SettingsViewModel @Inject constructor(
    private val sessions: SessionRepository,
    private val preferences: AppPreferences,
) : ViewModel() {
    val state: StateFlow<SettingsUiState> = combine(sessions.state, preferences.dynamicColor) { session, dynamic ->
        val connected = (session as? SessionState.Connected)?.session
        SettingsUiState(connected?.connection?.base?.toString().orEmpty(), connected?.account.orEmpty(), dynamic)
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), SettingsUiState())

    fun setDynamicColor(enabled: Boolean) {
        viewModelScope.launch { preferences.setDynamicColor(enabled) }
    }

    fun logout() = sessions.logout()
}

@Composable
fun SettingsRoute(viewModel: SettingsViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    SettingsScreen(state, viewModel::setDynamicColor, viewModel::logout)
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(state: SettingsUiState, onDynamicColor: (Boolean) -> Unit, onLogout: () -> Unit) {
    Scaffold(topBar = { TopAppBar(title = { Text(stringResource(R.string.settings)) }) }) { padding ->
        Column(Modifier.fillMaxSize().padding(padding).verticalScroll(rememberScrollState())) {
            ListItem(
                headlineContent = { Text(stringResource(R.string.server)) },
                supportingContent = { Text(state.server) },
                leadingContent = { Icon(painterResource(AppIcons.Server), contentDescription = null) },
            )
            if (state.account.isNotBlank()) ListItem(
                headlineContent = { Text(stringResource(R.string.account)) },
                supportingContent = { Text(state.account) },
            )
            HorizontalDivider()
            if (supportsDynamicColor) ListItem(
                headlineContent = { Text(stringResource(R.string.dynamic_color)) },
                supportingContent = { Text(stringResource(R.string.dynamic_color_summary)) },
                trailingContent = { Switch(checked = state.dynamicColor, onCheckedChange = null) },
                modifier = Modifier.toggleable(state.dynamicColor, role = Role.Switch, onValueChange = onDynamicColor),
            )
            HorizontalDivider()
            ListItem(
                headlineContent = { Text(stringResource(R.string.logout), color = MaterialTheme.colorScheme.error) },
                supportingContent = { Text(stringResource(R.string.logout_summary)) },
                leadingContent = { Icon(painterResource(AppIcons.Logout), contentDescription = null, tint = MaterialTheme.colorScheme.error) },
                modifier = Modifier.clickable(onClick = onLogout).testTag(SettingsTestTags.LOGOUT),
            )
        }
    }
}

object SettingsTestTags {
    const val LOGOUT = "logout"
}
