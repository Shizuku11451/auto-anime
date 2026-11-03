package com.autoanime.android.feature.discover

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.autoanime.android.core.designsystem.AppIcons
import com.autoanime.android.core.designsystem.CoverImage
import com.autoanime.android.core.designsystem.message
import com.autoanime.android.core.model.DiscoverSubject
import com.autoanime.android.core.model.displayName

@Composable
fun DiscoverRoute(onOpenSubject: (DiscoverSubject) -> Unit, viewModel: DiscoverViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    DiscoverScreen(state, viewModel::setQuery, viewModel::search, onOpenSubject)
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DiscoverScreen(
    state: DiscoverUiState,
    onQuery: (String) -> Unit,
    onSearch: () -> Unit,
    onOpenSubject: (DiscoverSubject) -> Unit,
) {
    val focus = LocalFocusManager.current
    Scaffold(topBar = { TopAppBar(title = { Text(stringResource(R.string.discover)) }) }) { padding ->
        Column(Modifier.fillMaxSize().padding(padding)) {
            OutlinedTextField(
                value = state.query,
                onValueChange = onQuery,
                placeholder = { Text(stringResource(R.string.search_bangumi)) },
                leadingIcon = { Icon(painterResource(AppIcons.Search), contentDescription = null) },
                singleLine = true,
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
                keyboardActions = KeyboardActions(onSearch = { focus.clearFocus(); onSearch() }),
                modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp).testTag(DiscoverTestTags.SEARCH),
            )
            if (state.searching) LinearProgressIndicator(Modifier.fillMaxWidth().padding(top = 8.dp))
            state.error?.let { Text(it.message(), color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(16.dp)) }
            if (!state.resourcesEnabled) Text(
                stringResource(R.string.resources_disabled),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
            )
            when {
                state.searched.isNotEmpty() && state.subjects.isEmpty() && !state.searching -> Box(
                    Modifier.fillMaxSize(),
                    contentAlignment = Alignment.Center,
                ) { Text(stringResource(R.string.no_subjects), color = MaterialTheme.colorScheme.onSurfaceVariant) }
                else -> LazyColumn(Modifier.fillMaxSize().testTag(DiscoverTestTags.RESULTS)) {
                    items(state.subjects, key = { it.subject.id }, contentType = { "subject" }) { card ->
                        SubjectRow(card, enabled = state.resourcesEnabled, onClick = { onOpenSubject(card.subject) })
                        HorizontalDivider(Modifier.padding(start = 16.dp))
                    }
                }
            }
        }
    }
}

@Composable
private fun SubjectRow(card: SubjectCard, enabled: Boolean, onClick: () -> Unit) {
    val subject = card.subject
    Row(
        Modifier
            .fillMaxWidth()
            .clickable(enabled = enabled, onClick = onClick)
            .padding(horizontal = 16.dp, vertical = 12.dp)
            .testTag(DiscoverTestTags.subject(subject.id)),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        CoverImage(card.coverUrl, contentDescription = null, modifier = Modifier.size(width = 56.dp, height = 80.dp).clip(RoundedCornerShape(8.dp)))
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(subject.displayName, style = MaterialTheme.typography.titleSmall, maxLines = 2, overflow = TextOverflow.Ellipsis)
            if (subject.name_cn.isNotBlank() && subject.name.isNotBlank() && subject.name != subject.name_cn) Text(
                subject.name,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            val meta = listOfNotNull(
                subject.date.takeIf(String::isNotBlank),
                subject.rating.takeIf { it > 0 }?.let { "★ %.1f".format(java.util.Locale.ROOT, it) },
            ).joinToString(" · ")
            if (meta.isNotEmpty()) Text(meta, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

@Composable
fun SubjectPlaceholder() {
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Text(stringResource(R.string.select_subject), color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

object DiscoverTestTags {
    const val SEARCH = "discover_search"
    const val RESULTS = "discover_results"
    const val SUBSCRIBE = "subscribe_version"
    const val CONFIRM_SUBSCRIBE = "confirm_subscribe"
    fun subject(id: Long) = "subject_$id"
}
