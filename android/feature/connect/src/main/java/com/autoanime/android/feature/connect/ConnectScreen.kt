package com.autoanime.android.feature.connect

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.relocation.BringIntoViewRequester
import androidx.compose.foundation.relocation.bringIntoViewRequester
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.snapshotFlow
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.autoanime.android.core.data.SessionState
import com.autoanime.android.core.designsystem.AppIcons
import com.autoanime.android.core.designsystem.message
import com.autoanime.android.core.network.InvalidServerUrl

@Composable
fun ConnectRoute(viewModel: ConnectViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val signedOut = state as? SessionState.SignedOut ?: SessionState.SignedOut()
    ConnectScreen(
        state = signedOut,
        onConnect = viewModel::connect,
    )
}

@Composable
fun ConnectScreen(
    state: SessionState.SignedOut,
    onConnect: (url: String, allowHttp: Boolean, account: String, password: String) -> InvalidServerUrl.Reason?,
) {
    // Prefilled once per visit; later state updates must not overwrite what the user is typing.
    var url by rememberSaveable { mutableStateOf(state.form?.url.orEmpty()) }
    var allowHttp by rememberSaveable { mutableStateOf(state.form?.allowHttp ?: false) }
    var account by rememberSaveable { mutableStateOf(state.form?.account.orEmpty()) }
    // The password is deliberately not saveable: it must not land in the saved-state bundle.
    var password by remember { mutableStateOf("") }
    var invalid by remember { mutableStateOf<InvalidServerUrl.Reason?>(null) }
    var passwordFocused by remember { mutableStateOf(false) }
    val connectButton = remember { BringIntoViewRequester() }
    val ime = WindowInsets.ime
    val density = LocalDensity.current
    // Focusing the password only scrolls the field into view; follow the keyboard as it opens so
    // the connect button below it is not left hidden.
    LaunchedEffect(passwordFocused) {
        if (passwordFocused) snapshotFlow { ime.getBottom(density) }.collect { connectButton.bringIntoView() }
    }
    val submit = {
        if (!state.connecting) {
            invalid = onConnect(url, allowHttp, account, password)
            if (invalid == null) password = ""
        }
    }

    Scaffold { padding ->
        Column(
            Modifier
                .fillMaxSize()
                .padding(padding)
                .consumeWindowInsets(padding)
                .imePadding()
                .verticalScroll(rememberScrollState())
                .padding(24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center,
        ) {
            Column(Modifier.widthIn(max = 480.dp).fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                Icon(
                    painterResource(AppIcons.Server),
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.primary,
                    modifier = Modifier.size(40.dp),
                )
                Text(stringResource(R.string.connect_title), style = MaterialTheme.typography.headlineMedium)
                OutlinedTextField(
                    value = url,
                    onValueChange = { url = it; invalid = null },
                    label = { Text(stringResource(R.string.server_url)) },
                    placeholder = { Text("https://nas.example/") },
                    singleLine = true,
                    isError = invalid != null,
                    supportingText = invalid?.let { { Text(it.message()) } },
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri, imeAction = ImeAction.Next),
                    modifier = Modifier.fillMaxWidth().testTag(ConnectTestTags.SERVER_URL),
                )
                Row(
                    Modifier
                        .fillMaxWidth()
                        .toggleable(value = allowHttp, role = Role.Switch, onValueChange = { allowHttp = it; invalid = null })
                        .testTag(ConnectTestTags.ALLOW_HTTP),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(16.dp),
                ) {
                    Column(Modifier.weight(1f)) {
                        Text(stringResource(R.string.allow_http), style = MaterialTheme.typography.bodyLarge)
                        Text(
                            stringResource(R.string.allow_http_warning),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    Switch(checked = allowHttp, onCheckedChange = null)
                }
                OutlinedTextField(
                    value = account,
                    onValueChange = { account = it },
                    label = { Text(stringResource(R.string.username)) },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(imeAction = ImeAction.Next),
                    modifier = Modifier.fillMaxWidth().testTag(ConnectTestTags.USERNAME),
                )
                OutlinedTextField(
                    value = password,
                    onValueChange = { password = it },
                    label = { Text(stringResource(R.string.password)) },
                    singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Done),
                    keyboardActions = KeyboardActions(onDone = { submit() }),
                    modifier = Modifier
                        .fillMaxWidth()
                        .onFocusChanged { passwordFocused = it.isFocused }
                        .testTag(ConnectTestTags.PASSWORD),
                )
                state.error?.let {
                    Text(it.message(), color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium)
                }
                Button(
                    onClick = submit,
                    enabled = !state.connecting,
                    modifier = Modifier
                        .fillMaxWidth()
                        .bringIntoViewRequester(connectButton)
                        .testTag(ConnectTestTags.CONNECT),
                ) {
                    if (state.connecting) CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
                    else Text(stringResource(R.string.connect))
                }
            }
        }
    }
}

@Composable
private fun InvalidServerUrl.Reason.message(): String = stringResource(
    when (this) {
        InvalidServerUrl.Reason.Malformed -> R.string.url_malformed
        InvalidServerUrl.Reason.Credentials -> R.string.url_credentials
        InvalidServerUrl.Reason.QueryOrFragment -> R.string.url_query
        InvalidServerUrl.Reason.PathWithoutSlash -> R.string.url_trailing_slash
        InvalidServerUrl.Reason.HttpNotAllowed -> R.string.url_http_not_allowed
    },
)

object ConnectTestTags {
    const val SERVER_URL = "server_url"
    const val ALLOW_HTTP = "allow_http"
    const val USERNAME = "username"
    const val PASSWORD = "password"
    const val CONNECT = "connect"
}
