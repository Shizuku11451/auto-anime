package com.autoanime.android.feature.library

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.grid.rememberLazyGridState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.autoanime.android.core.designsystem.CoverImage
import com.autoanime.android.core.designsystem.ErrorState
import com.autoanime.android.core.designsystem.LoadingState
import com.autoanime.android.core.designsystem.message

@Composable
fun LibraryRoute(onOpenAnime: (Long) -> Unit, viewModel: LibraryViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    LibraryScreen(state, onOpenAnime, onRefresh = viewModel::refresh, onLoadMore = viewModel::loadMore)
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun LibraryScreen(
    state: LibraryUiState,
    onOpenAnime: (Long) -> Unit,
    onRefresh: () -> Unit,
    onLoadMore: () -> Unit,
) {
    Scaffold(topBar = { TopAppBar(title = { Text(stringResource(R.string.library)) }) }) { padding ->
        val body = Modifier.fillMaxSize().padding(padding)
        when {
            state.items.isEmpty() && state.error != null -> ErrorState(state.error, onRetry = onRefresh, modifier = body)
            state.items.isEmpty() && state.loading -> LoadingState(body)
            state.items.isEmpty() -> Box(body, contentAlignment = Alignment.Center) {
                Text(stringResource(R.string.library_empty), color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            else -> PullToRefreshBox(
                isRefreshing = state.refreshing,
                onRefresh = onRefresh,
                modifier = body,
            ) {
                AnimeGrid(state, onOpenAnime, onLoadMore)
            }
        }
    }
}

@Composable
private fun AnimeGrid(state: LibraryUiState, onOpenAnime: (Long) -> Unit, onLoadMore: () -> Unit) {
    val grid = rememberLazyGridState()
    val nearEnd by remember {
        derivedStateOf {
            val last = grid.layoutInfo.visibleItemsInfo.lastOrNull()?.index ?: 0
            last >= grid.layoutInfo.totalItemsCount - 6
        }
    }
    LaunchedEffect(nearEnd, state.endReached, state.items.size) {
        if (nearEnd && !state.endReached && state.error == null) onLoadMore()
    }
    LazyVerticalGrid(
        columns = GridCells.Adaptive(minSize = 128.dp),
        state = grid,
        contentPadding = PaddingValues(16.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
        modifier = Modifier.fillMaxSize().testTag(LibraryTestTags.GRID),
    ) {
        items(state.items, key = { it.id }, contentType = { "anime" }) { anime ->
            AnimeCardItem(anime, onClick = { onOpenAnime(anime.id) })
        }
        if (!state.endReached || state.error != null) {
            item(span = { GridItemSpan(maxLineSpan) }, contentType = "footer") {
                Box(Modifier.fillMaxWidth().padding(16.dp), contentAlignment = Alignment.Center) {
                    val error = state.error
                    if (error != null) TextButton(onClick = onLoadMore) { Text(error.message()) }
                    else CircularProgressIndicator()
                }
            }
        }
    }
}

@Composable
private fun AnimeCardItem(anime: AnimeCard, onClick: () -> Unit) {
    Column(
        Modifier.clip(RoundedCornerShape(12.dp)).clickable(onClick = onClick),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        CoverImage(
            url = anime.coverUrl,
            contentDescription = null,
            modifier = Modifier.fillMaxWidth().aspectRatio(2f / 3f).clip(RoundedCornerShape(12.dp)),
        )
        Text(
            anime.title,
            style = MaterialTheme.typography.titleSmall,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
        )
        val subtitle = anime.subtitle ?: anime.season.takeIf { it > 0 }?.let { stringResource(R.string.season_number, it) }
        if (subtitle != null) Text(
            subtitle,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

object LibraryTestTags {
    /** Also exposed as a resource id for the scroll benchmark. */
    const val GRID = "anime_list"
}
