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
import com.autoanime.android.core.model.MediaItem
import com.autoanime.android.core.designsystem.R as DesignR

@Composable
fun DetailRoute(
    animeId: Long,
    onPlay: (itemId: Long, title: String) -> Unit,
    onBack: (() -> Unit)?,
) {
    val viewModel = hiltViewModel<DetailViewModel, DetailViewModel.Factory>(
        key = "anime-$animeId",
        creationCallback = { it.create(animeId) },
    )
    val state by viewModel.state.collectAsStateWithLifecycle()
    DetailScreen(state, onPlay, onBack, onRetry = viewModel::load)
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DetailScreen(
    state: DetailUiState,
    onPlay: (itemId: Long, title: String) -> Unit,
    onBack: (() -> Unit)?,
    onRetry: () -> Unit,
) {
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(state.title, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                navigationIcon = {
                    if (onBack != null) IconButton(onClick = onBack) {
                        Icon(painterResource(AppIcons.Back), contentDescription = stringResource(DesignR.string.back))
                    }
                },
            )
        },
    ) { padding ->
        val body = Modifier.fillMaxSize().padding(padding)
        when {
            !state.loaded && state.error != null -> ErrorState(state.error, onRetry, body)
            !state.loaded -> LoadingState(body)
            else -> LazyColumn(body, contentPadding = PaddingValues(bottom = 24.dp)) {
                item(contentType = "header") { Header(state) }
                if (state.episodes.isEmpty()) item(contentType = "empty") {
                    Text(
                        stringResource(R.string.no_saved_episodes),
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(16.dp),
                    )
                }
                items(state.episodes, key = { it.id }, contentType = { "episode" }) { item ->
                    val label = item.label()
                    ListItem(
                        headlineContent = { Text(label, maxLines = 2, overflow = TextOverflow.Ellipsis) },
                        supportingContent = item.quality.takeIf { it.isNotBlank() }?.let { { Text(it) } },
                        trailingContent = { Icon(painterResource(AppIcons.Play), contentDescription = null) },
                        modifier = Modifier.clickable { onPlay(item.id, label) },
                    )
                }
            }
        }
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
