package com.autoanime.android.navigation

import androidx.activity.compose.ReportDrawnWhen
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.Icon
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.adaptive.ExperimentalMaterial3AdaptiveApi
import androidx.compose.material3.adaptive.currentWindowAdaptiveInfo
import androidx.compose.material3.adaptive.navigation3.ListDetailSceneStrategy
import androidx.compose.material3.adaptive.navigation3.rememberListDetailSceneStrategy
import androidx.compose.material3.adaptive.navigationsuite.NavigationSuiteItem
import androidx.compose.material3.adaptive.navigationsuite.NavigationSuiteScaffold
import androidx.compose.material3.adaptive.navigationsuite.NavigationSuiteScaffoldDefaults
import androidx.compose.material3.adaptive.navigationsuite.NavigationSuiteType
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.key
import androidx.compose.ui.ExperimentalComposeUiApi
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTagsAsResourceId
import androidx.lifecycle.viewmodel.navigation3.rememberViewModelStoreNavEntryDecorator
import androidx.navigation3.runtime.NavBackStack
import androidx.navigation3.runtime.NavKey
import androidx.navigation3.runtime.entryProvider
import androidx.navigation3.runtime.rememberNavBackStack
import androidx.navigation3.runtime.rememberSaveableStateHolderNavEntryDecorator
import androidx.navigation3.ui.NavDisplay
import com.autoanime.android.core.data.SessionState
import com.autoanime.android.core.designsystem.AppIcons
import com.autoanime.android.core.designsystem.LocalCoverImageLoader
import com.autoanime.android.feature.connect.ConnectRoute
import com.autoanime.android.feature.downloads.DownloadsRoute
import com.autoanime.android.feature.library.DetailPlaceholder
import com.autoanime.android.feature.library.DetailRoute
import com.autoanime.android.feature.library.LibraryRoute
import com.autoanime.android.feature.player.PlayerRoute
import com.autoanime.android.feature.settings.SettingsRoute
import kotlinx.serialization.Serializable
import com.autoanime.android.feature.downloads.R as DownloadsR
import com.autoanime.android.feature.library.R as LibraryR
import com.autoanime.android.feature.settings.R as SettingsR

@Serializable data object LibraryKey : NavKey
@Serializable data class DetailKey(val animeId: Long) : NavKey
@Serializable data class PlayerKey(val itemId: Long, val title: String) : NavKey
@Serializable data object DownloadsKey : NavKey
@Serializable data object SettingsKey : NavKey

@OptIn(ExperimentalComposeUiApi::class)
@Composable
fun AutoAnimeRoot(state: SessionState) {
    // Resource ids let UI Automator (benchmarks) find Compose test tags.
    Box(Modifier.fillMaxSize().semantics { testTagsAsResourceId = true }) {
        when (state) {
            SessionState.Restoring -> Surface(Modifier.fillMaxSize()) {}
            is SessionState.SignedOut -> ConnectRoute()
            // A new session starts from a fresh back stack, so no screen outlives its server.
            is SessionState.Connected -> key(state.session.id) {
                CompositionLocalProvider(LocalCoverImageLoader provides state.session.imageLoader) {
                    MainNavigation()
                }
            }
        }
    }
    // Marks UI readiness; library data and covers are measured separately.
    ReportDrawnWhen { state != SessionState.Restoring }
}

@OptIn(ExperimentalMaterial3AdaptiveApi::class)
@Composable
private fun MainNavigation() {
    val backStack = rememberNavBackStack(LibraryKey)
    val inSettings = backStack.any { it is SettingsKey }
    val inDownloads = backStack.any { it is DownloadsKey }
    val suiteType = if (backStack.last() is PlayerKey) NavigationSuiteType.None
    else NavigationSuiteScaffoldDefaults.navigationSuiteType(currentWindowAdaptiveInfo())

    NavigationSuiteScaffold(
        navigationSuiteType = suiteType,
        navigationItems = {
            NavigationSuiteItem(
                selected = !inSettings && !inDownloads,
                onClick = { backStack.resetTo(LibraryKey) },
                icon = { Icon(painterResource(AppIcons.Library), contentDescription = null) },
                label = { Text(stringResource(LibraryR.string.library)) },
                navigationSuiteType = suiteType,
            )
            NavigationSuiteItem(
                selected = inDownloads,
                onClick = { if (!inDownloads) backStack.resetTo(LibraryKey, DownloadsKey) },
                icon = { Icon(painterResource(AppIcons.Download), contentDescription = null) },
                label = { Text(stringResource(DownloadsR.string.downloads)) },
                navigationSuiteType = suiteType,
                modifier = Modifier.testTag(NavTestTags.DOWNLOADS),
            )
            NavigationSuiteItem(
                selected = inSettings,
                onClick = { if (!inSettings) backStack.resetTo(LibraryKey, SettingsKey) },
                icon = { Icon(painterResource(AppIcons.Settings), contentDescription = null) },
                label = { Text(stringResource(SettingsR.string.settings)) },
                navigationSuiteType = suiteType,
            )
        },
    ) {
        NavDisplay(
            backStack = backStack,
            onBack = { backStack.removeLastOrNull() },
            entryDecorators = listOf(
                rememberSaveableStateHolderNavEntryDecorator(),
                rememberViewModelStoreNavEntryDecorator(),
            ),
            sceneStrategies = listOf(rememberListDetailSceneStrategy()),
            entryProvider = entryProvider {
                entry<LibraryKey>(metadata = ListDetailSceneStrategy.listPane(detailPlaceholder = { DetailPlaceholder() })) {
                    LibraryRoute(onOpenAnime = { id -> backStack.openDetail(id) })
                }
                entry<DetailKey>(metadata = ListDetailSceneStrategy.detailPane()) { key ->
                    DetailRoute(
                        animeId = key.animeId,
                        onPlay = { itemId, title -> backStack.add(PlayerKey(itemId, title)) },
                        onBack = { backStack.removeLastOrNull() },
                    )
                }
                entry<PlayerKey> { key ->
                    PlayerRoute(key.itemId, key.title, onClose = { backStack.removeLastOrNull() })
                }
                entry<DownloadsKey> {
                    DownloadsRoute(onPlay = { itemId, title -> backStack.add(PlayerKey(itemId, title)) })
                }
                entry<SettingsKey> { SettingsRoute() }
            },
        )
    }
}

/** In the two-pane layout a new selection replaces the open detail instead of stacking. */
private fun NavBackStack<NavKey>.openDetail(animeId: Long) {
    val detail = DetailKey(animeId)
    if (last() is DetailKey) set(lastIndex, detail) else add(detail)
}

private fun NavBackStack<NavKey>.resetTo(vararg keys: NavKey) {
    clear()
    addAll(keys)
}

object NavTestTags {
    const val DOWNLOADS = "nav_downloads"
}
