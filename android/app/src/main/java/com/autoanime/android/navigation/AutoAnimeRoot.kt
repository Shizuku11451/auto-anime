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
import com.autoanime.android.core.model.displayName
import com.autoanime.android.feature.discover.DiscoverRoute
import com.autoanime.android.feature.discover.SubjectPlaceholder
import com.autoanime.android.feature.discover.SubjectRoute
import com.autoanime.android.feature.downloads.DownloadsRoute
import com.autoanime.android.feature.feeds.FeedEditorRoute
import com.autoanime.android.feature.feeds.FeedPlaceholder
import com.autoanime.android.feature.feeds.FeedsRoute
import com.autoanime.android.feature.library.AnimeEditRoute
import com.autoanime.android.feature.library.BindRoute
import com.autoanime.android.feature.settings.ServerSettingsRoute
import com.autoanime.android.feature.library.DetailPlaceholder
import com.autoanime.android.feature.library.DetailRoute
import com.autoanime.android.feature.library.LibraryRoute
import com.autoanime.android.feature.player.PlayerRoute
import com.autoanime.android.feature.settings.SettingsRoute
import kotlinx.serialization.Serializable
import com.autoanime.android.feature.discover.R as DiscoverR
import com.autoanime.android.feature.downloads.R as DownloadsR
import com.autoanime.android.feature.feeds.R as FeedsR
import com.autoanime.android.feature.library.R as LibraryR
import com.autoanime.android.feature.settings.R as SettingsR

@Serializable data object LibraryKey : NavKey
@Serializable data class DetailKey(val animeId: Long) : NavKey
@Serializable data class PlayerKey(val itemId: Long, val title: String) : NavKey
@Serializable data class AnimeEditKey(val animeId: Long) : NavKey
@Serializable data class BindKey(val animeId: Long) : NavKey
@Serializable data object DiscoverKey : NavKey
@Serializable data class SubjectKey(val subjectId: Long, val name: String) : NavKey
@Serializable data object FeedsKey : NavKey
/** A null [feedId] adds a new subscription. */
@Serializable data class FeedEditKey(val feedId: Long?) : NavKey
@Serializable data object DownloadsKey : NavKey
@Serializable data object ServerSettingsKey : NavKey
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

/**
 * Top-level destinations. The library is always the root of the back stack; any other tab sits
 * directly on top of it, so Back from a tab returns to the library.
 */
private enum class Tab(val root: NavKey) {
    Library(LibraryKey),
    Discover(DiscoverKey),
    Feeds(FeedsKey),
    Downloads(DownloadsKey),
    Settings(SettingsKey),
}

@OptIn(ExperimentalMaterial3AdaptiveApi::class)
@Composable
private fun MainNavigation() {
    val backStack = rememberNavBackStack(LibraryKey)
    val current = Tab.entries.lastOrNull { it != Tab.Library && it.root in backStack } ?: Tab.Library
    val suiteType = if (backStack.last() is PlayerKey) NavigationSuiteType.None
    else NavigationSuiteScaffoldDefaults.navigationSuiteType(currentWindowAdaptiveInfo())
    val play: (Long, String) -> Unit = { itemId, title -> backStack.add(PlayerKey(itemId, title)) }

    NavigationSuiteScaffold(
        navigationSuiteType = suiteType,
        navigationItems = {
            Tab.entries.forEach { tab ->
                NavigationSuiteItem(
                    selected = current == tab,
                    // Selecting the current tab again returns to its first screen.
                    onClick = { if (tab == Tab.Library) backStack.resetTo(LibraryKey) else backStack.resetTo(LibraryKey, tab.root) },
                    icon = { Icon(painterResource(tab.icon()), contentDescription = null) },
                    label = { Text(stringResource(tab.label())) },
                    navigationSuiteType = suiteType,
                    modifier = Modifier.testTag(NavTestTags.tab(tab.name)),
                )
            }
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
                    LibraryRoute(onOpenAnime = { id -> backStack.openPane(DetailKey(id)) })
                }
                entry<DetailKey>(metadata = ListDetailSceneStrategy.detailPane()) { key ->
                    DetailRoute(
                        animeId = key.animeId,
                        onPlay = play,
                        onBack = { backStack.removeLastOrNull() },
                        onEdit = { id -> backStack.add(AnimeEditKey(id)) },
                        onBind = { id -> backStack.add(BindKey(id)) },
                    )
                }
                entry<AnimeEditKey>(metadata = ListDetailSceneStrategy.detailPane()) { key ->
                    AnimeEditRoute(key.animeId, onDone = { backStack.removeLastOrNull() })
                }
                entry<BindKey>(metadata = ListDetailSceneStrategy.detailPane()) { key ->
                    BindRoute(key.animeId, onDone = { backStack.removeLastOrNull() })
                }
                entry<PlayerKey> { key ->
                    PlayerRoute(key.itemId, key.title, onClose = { backStack.removeLastOrNull() })
                }
                entry<DiscoverKey>(metadata = ListDetailSceneStrategy.listPane(detailPlaceholder = { SubjectPlaceholder() })) {
                    DiscoverRoute(onOpenSubject = { subject -> backStack.openPane(SubjectKey(subject.id, subject.displayName)) })
                }
                entry<SubjectKey>(metadata = ListDetailSceneStrategy.detailPane()) { key ->
                    SubjectRoute(key.subjectId, key.name, onBack = { backStack.removeLastOrNull() })
                }
                entry<FeedsKey>(metadata = ListDetailSceneStrategy.listPane(detailPlaceholder = { FeedPlaceholder() })) {
                    FeedsRoute(onEdit = { id -> backStack.openPane(FeedEditKey(id)) })
                }
                entry<FeedEditKey>(metadata = ListDetailSceneStrategy.detailPane()) { key ->
                    FeedEditorRoute(key.feedId, onDone = { backStack.removeLastOrNull() })
                }
                entry<DownloadsKey> { DownloadsRoute(onPlay = play) }
                entry<SettingsKey> { SettingsRoute(onServerSettings = { backStack.add(ServerSettingsKey) }) }
                entry<ServerSettingsKey> { ServerSettingsRoute(onBack = { backStack.removeLastOrNull() }) }
            },
        )
    }
}

private fun Tab.icon(): Int = when (this) {
    Tab.Library -> AppIcons.Library
    Tab.Discover -> AppIcons.Discover
    Tab.Feeds -> AppIcons.Feeds
    Tab.Downloads -> AppIcons.Download
    Tab.Settings -> AppIcons.Settings
}

private fun Tab.label(): Int = when (this) {
    Tab.Library -> LibraryR.string.library
    Tab.Discover -> DiscoverR.string.discover
    Tab.Feeds -> FeedsR.string.feeds
    Tab.Downloads -> DownloadsR.string.downloads
    Tab.Settings -> SettingsR.string.settings
}

/**
 * In the two-pane layout a new selection replaces the open detail of the same kind instead of
 * stacking, so Back leaves the list rather than walking through earlier selections.
 */
private inline fun <reified T : NavKey> NavBackStack<NavKey>.openPane(key: T) {
    if (last() is T) set(lastIndex, key) else add(key)
}

private fun NavBackStack<NavKey>.resetTo(vararg keys: NavKey) {
    clear()
    addAll(keys)
}

object NavTestTags {
    const val DOWNLOADS = "nav_Downloads"
    fun tab(name: String) = "nav_$name"
}
