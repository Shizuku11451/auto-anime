package com.autoanime.android.feature.feeds

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.autoanime.android.core.data.CallResult
import com.autoanime.android.core.data.IoDispatcher
import com.autoanime.android.core.data.SessionRepository
import com.autoanime.android.core.data.call
import com.autoanime.android.core.data.connected
import com.autoanime.android.core.model.AppError
import com.autoanime.android.core.model.Feed
import com.autoanime.android.core.model.FeedInput
import com.autoanime.android.core.model.PreviewInput
import com.autoanime.android.core.model.PreviewResult
import com.autoanime.android.core.model.SEASON_ANY
import com.autoanime.android.core.model.SEASON_UNMARKED
import com.autoanime.android.core.model.isAggregate
import dagger.assisted.Assisted
import dagger.assisted.AssistedFactory
import dagger.assisted.AssistedInject
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull

enum class SeasonMode { Any, Unmarked, Specific }

/** Editable fields; [season] is text so a half-typed number is not lost. */
data class FeedForm(
    val name: String = "",
    val url: String = "",
    val targetTitle: String = "",
    val include: String = "",
    val exclude: String = "",
    val autoDownload: Boolean = true,
    val seasonMode: SeasonMode = SeasonMode.Any,
    val season: String = "1",
    val allowUnknownSeason: Boolean = false,
)

enum class FormProblem { Name, Url, Season }

data class FeedEditorUiState(
    val isNew: Boolean = true,
    /** Editing: the stored subscription is still loading or could not be found. */
    val loading: Boolean = false,
    val loadError: AppError? = null,
    val aggregate: Boolean = false,
    val form: FeedForm = FeedForm(),
    val problem: FormProblem? = null,
    val previewing: Boolean = false,
    /** Matches the current form; cleared whenever an input that affects matching changes. */
    val preview: PreviewResult? = null,
    val previewError: AppError? = null,
    val saving: Boolean = false,
    val saveError: AppError? = null,
    /** Set once saved; the screen closes and reports how many items the first sync added, if any. */
    val saved: SaveOutcome? = null,
)

data class SaveOutcome(val created: Boolean, val added: Int?, val storageWarning: String)

/**
 * Adds a manual RSS subscription or edits an existing one. The address and storage location are
 * not editable here: the address is fixed after creation and storage paths belong to the server.
 */
@HiltViewModel(assistedFactory = FeedEditorViewModel.Factory::class)
class FeedEditorViewModel @AssistedInject constructor(
    @Assisted private val feedId: Long?,
    private val sessions: SessionRepository,
    @IoDispatcher private val io: CoroutineDispatcher,
) : ViewModel() {
    @AssistedFactory
    interface Factory {
        fun create(feedId: Long?): FeedEditorViewModel
    }

    private val _state = MutableStateFlow(FeedEditorUiState(isNew = feedId == null, loading = feedId != null))
    val state: StateFlow<FeedEditorUiState> = _state.asStateFlow()
    private var stored: Feed? = null
    private var previewJob: Job? = null
    private var previewToken = 0

    init {
        if (feedId != null) load(feedId)
    }

    fun retryLoad() {
        feedId?.let(::load)
    }

    private fun load(id: Long) {
        val owner = sessions.connected ?: return
        _state.update { it.copy(loading = true, loadError = null) }
        viewModelScope.launch {
            when (val result = sessions.call(owner, io) { feeds() }) {
                is CallResult.Success -> {
                    val feed = result.value.firstOrNull { it.id == id }
                    if (feed == null) {
                        _state.update { it.copy(loading = false, loadError = AppError.Http(404)) }
                        return@launch
                    }
                    stored = feed
                    _state.update { it.copy(loading = false, aggregate = feed.isAggregate, form = feed.toForm()) }
                }
                is CallResult.Failure -> _state.update { it.copy(loading = false, loadError = result.error) }
                CallResult.Stale -> Unit
            }
        }
    }

    fun update(transform: (FeedForm) -> FeedForm) {
        val before = _state.value.form
        val after = transform(before)
        val affectsMatching = after.copy(name = before.name, targetTitle = before.targetTitle, autoDownload = before.autoDownload) != before
        if (affectsMatching) {
            previewJob?.cancel()
            previewToken++
        }
        _state.update {
            it.copy(
                form = after,
                problem = null,
                saveError = null,
                preview = if (affectsMatching) null else it.preview,
                previewError = if (affectsMatching) null else it.previewError,
                previewing = if (affectsMatching) false else it.previewing,
            )
        }
    }

    fun preview() {
        val owner = sessions.connected ?: return
        val form = _state.value.form
        val url = if (_state.value.isNew) form.url.trim() else stored?.url ?: return
        validate(form, checkName = false)?.let { problem -> _state.update { it.copy(problem = problem) }; return }
        val seasonFilter = form.seasonFilter() ?: return
        previewJob?.cancel()
        val token = ++previewToken
        _state.update { it.copy(previewing = true, preview = null, previewError = null) }
        previewJob = viewModelScope.launch {
            val result = sessions.call(owner, io) {
                preview(PreviewInput(url, form.include.trim(), form.exclude.trim(), seasonFilter, form.allowUnknownSeason))
            }
            if (token != previewToken) return@launch
            when (result) {
                is CallResult.Success -> _state.update { it.copy(previewing = false, preview = result.value) }
                is CallResult.Failure -> _state.update { it.copy(previewing = false, previewError = result.error) }
                CallResult.Stale -> Unit
            }
        }
    }

    fun save() {
        val owner = sessions.connected ?: return
        val current = _state.value
        if (current.saving || current.loading) return
        val form = current.form
        validate(form, checkName = true)?.let { problem -> _state.update { it.copy(problem = problem) }; return }
        val seasonFilter = form.seasonFilter() ?: return
        val input = FeedInput(
            name = form.name.trim(),
            include_words = form.include.trim(),
            exclude_words = form.exclude.trim(),
            auto_download = form.autoDownload,
            url = if (current.isNew) form.url.trim() else null,
            target_title = form.targetTitle.trim(),
            season_filter = seasonFilter,
            allow_unknown_season = form.allowUnknownSeason,
        )
        _state.update { it.copy(saving = true, saveError = null) }
        viewModelScope.launch {
            val result = sessions.call(owner, io) {
                if (current.isNew) {
                    val created = createFeed(input)
                    // Like the web client, a new subscription syncs once so its releases appear now.
                    // A failed first sync is not a failed save; the subscription syncs again on schedule.
                    val added = try {
                        syncFeed(created.id).added
                    } catch (e: CancellationException) {
                        throw e
                    } catch (e: Exception) {
                        null
                    }
                    SaveOutcome(created = true, added = added, storageWarning = created.storage_warning)
                } else {
                    val updated = updateFeed(requireNotNull(feedId), input)
                    SaveOutcome(created = false, added = null, storageWarning = updated.storage_warning)
                }
            }
            when (result) {
                is CallResult.Success -> _state.update { it.copy(saving = false, saved = result.value) }
                is CallResult.Failure -> _state.update { it.copy(saving = false, saveError = result.error) }
                CallResult.Stale -> Unit
            }
        }
    }

    private fun validate(form: FeedForm, checkName: Boolean): FormProblem? = when {
        checkName && form.name.isBlank() -> FormProblem.Name
        _state.value.isNew && !form.url.trim().isRssUrl() -> FormProblem.Url
        form.seasonFilter() == null -> FormProblem.Season
        else -> null
    }
}

private fun FeedForm.seasonFilter(): Int? = when (seasonMode) {
    SeasonMode.Any -> SEASON_ANY
    SeasonMode.Unmarked -> SEASON_UNMARKED
    SeasonMode.Specific -> season.trim().toIntOrNull()?.takeIf { it in 1..999 }
}

private fun String.isRssUrl(): Boolean = toHttpUrlOrNull()?.let { it.host.isNotEmpty() } == true

private fun Feed.toForm() = FeedForm(
    name = name,
    targetTitle = target_title,
    include = include_words,
    exclude = exclude_words,
    autoDownload = auto_download,
    seasonMode = when {
        season_filter == SEASON_ANY -> SeasonMode.Any
        season_filter == SEASON_UNMARKED -> SeasonMode.Unmarked
        else -> SeasonMode.Specific
    },
    season = if (season_filter > 0) season_filter.toString() else "1",
    allowUnknownSeason = allow_unknown_season,
)
