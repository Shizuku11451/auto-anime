package com.autoanime.android.feature.library

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.autoanime.android.core.data.CallResult
import com.autoanime.android.core.data.IoDispatcher
import com.autoanime.android.core.data.SessionRepository
import com.autoanime.android.core.data.call
import com.autoanime.android.core.data.connected
import com.autoanime.android.core.model.AnimeDetail
import com.autoanime.android.core.model.AnimeUpdateInput
import com.autoanime.android.core.model.AppError
import com.autoanime.android.core.model.MetaCandidate
import dagger.assisted.Assisted
import dagger.assisted.AssistedFactory
import dagger.assisted.AssistedInject
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class AnimeForm(
    val title: String = "",
    val searchName: String = "",
    val season: String = "1",
    /** 0 follows Bangumi; 1-7 is a manual update day. */
    val updateWeekday: Int = 0,
    val include: String = "",
    val exclude: String = "",
)

data class AnimeEditUiState(
    val loading: Boolean = true,
    val loadError: AppError? = null,
    val form: AnimeForm = AnimeForm(),
    val titleMissing: Boolean = false,
    val seasonInvalid: Boolean = false,
    val includeMissing: Boolean = false,
    val saving: Boolean = false,
    val saveError: AppError? = null,
    val done: EditOutcome? = null,
)

/** [matched] is set when the rules were applied to existing items too. */
data class EditOutcome(val matched: Int?, val storageWarning: String)

/**
 * Edits an anime's name, search name, default season, update day and matching rules. The storage
 * location is sent back unchanged: it is a server path and moving media needs the web workflow.
 */
@HiltViewModel(assistedFactory = AnimeEditViewModel.Factory::class)
class AnimeEditViewModel @AssistedInject constructor(
    @Assisted private val animeId: Long,
    private val sessions: SessionRepository,
    @IoDispatcher private val io: CoroutineDispatcher,
) : ViewModel() {
    @AssistedFactory
    interface Factory {
        fun create(animeId: Long): AnimeEditViewModel
    }

    private val _state = MutableStateFlow(AnimeEditUiState())
    val state: StateFlow<AnimeEditUiState> = _state.asStateFlow()
    private var stored: AnimeDetail? = null

    init {
        load()
    }

    fun load() {
        val owner = sessions.connected ?: return
        _state.update { it.copy(loading = true, loadError = null) }
        viewModelScope.launch {
            when (val result = sessions.call(owner, io) { detail(animeId) }) {
                is CallResult.Success -> {
                    val anime = result.value
                    stored = anime
                    _state.update {
                        it.copy(
                            loading = false,
                            form = AnimeForm(
                                title = anime.title,
                                searchName = anime.search_name,
                                season = anime.season.coerceAtLeast(1).toString(),
                                updateWeekday = anime.update_weekday.takeIf { d -> d in 0..7 } ?: 0,
                                include = anime.include_rules,
                                exclude = anime.exclude_rules,
                            ),
                        )
                    }
                }
                is CallResult.Failure -> _state.update { it.copy(loading = false, loadError = result.error) }
                CallResult.Stale -> Unit
            }
        }
    }

    fun update(transform: (AnimeForm) -> AnimeForm) = _state.update {
        it.copy(form = transform(it.form), titleMissing = false, seasonInvalid = false, includeMissing = false, saveError = null)
    }

    fun save() = submit(applyRules = false)

    /** Saves first, so existing items are matched against the rules on screen. */
    fun saveAndApply() = submit(applyRules = true)

    private fun submit(applyRules: Boolean) {
        val owner = sessions.connected ?: return
        val anime = stored ?: return
        val current = _state.value
        if (current.saving) return
        val form = current.form
        val season = form.season.trim().toIntOrNull()?.takeIf { it in 1..999 }
        if (form.title.isBlank() || season == null || (applyRules && form.include.isBlank())) {
            _state.update {
                it.copy(titleMissing = form.title.isBlank(), seasonInvalid = season == null, includeMissing = applyRules && form.include.isBlank())
            }
            return
        }
        val input = AnimeUpdateInput(
            title = form.title.trim(),
            search_name = form.searchName.trim(),
            season = season,
            update_weekday = form.updateWeekday,
            storage_dir = anime.storage_dir,
            include_rules = form.include.trim(),
            exclude_rules = form.exclude.trim(),
        )
        _state.update { it.copy(saving = true, saveError = null) }
        viewModelScope.launch {
            val result = sessions.call(owner, io) {
                val saved = updateAnime(animeId, input)
                val matched = if (applyRules) applyRules(animeId).matched else null
                EditOutcome(matched, saved.storage_warning)
            }
            when (result) {
                is CallResult.Success -> _state.update { it.copy(saving = false, done = result.value) }
                is CallResult.Failure -> _state.update { it.copy(saving = false, saveError = result.error) }
                CallResult.Stale -> Unit
            }
        }
    }
}

data class BindUiState(
    val query: String = "",
    val searching: Boolean = false,
    val searched: Boolean = false,
    val results: List<MetaCandidate> = emptyList(),
    val bound: Boolean = false,
    val queryTooShort: Boolean = false,
    val error: AppError? = null,
    /** The Bangumi ID being bound; 0 while unbinding. */
    val binding: Long? = null,
    val done: Boolean = false,
)

/** Binds an anime to a Bangumi subject (or removes the binding); the server then refreshes its metadata. */
@HiltViewModel(assistedFactory = BindViewModel.Factory::class)
class BindViewModel @AssistedInject constructor(
    @Assisted private val animeId: Long,
    private val sessions: SessionRepository,
    @IoDispatcher private val io: CoroutineDispatcher,
) : ViewModel() {
    @AssistedFactory
    interface Factory {
        fun create(animeId: Long): BindViewModel
    }

    private val _state = MutableStateFlow(BindUiState())
    val state: StateFlow<BindUiState> = _state.asStateFlow()
    private var searchJob: Job? = null
    private var token = 0

    init {
        val owner = sessions.connected
        if (owner != null) viewModelScope.launch {
            val result = sessions.call(owner, io) { detail(animeId) }
            if (result is CallResult.Success) {
                val anime = result.value
                val query = anime.search_name.ifBlank { anime.meta_name_cn }.ifBlank { cleanTitle(anime.title) }
                _state.update { it.copy(query = query, bound = anime.bangumi_id > 0) }
                if (query.trim().length >= MIN_QUERY) search()
            }
        }
    }

    fun setQuery(text: String) = _state.update { it.copy(query = text, queryTooShort = false) }

    fun search() {
        val owner = sessions.connected ?: return
        val query = _state.value.query.trim()
        if (query.length < MIN_QUERY) {
            _state.update { it.copy(queryTooShort = true) }
            return
        }
        searchJob?.cancel()
        val current = ++token
        _state.update { it.copy(searching = true, error = null) }
        searchJob = viewModelScope.launch {
            val result = sessions.call(owner, io) { searchMetadata(query) }
            if (current != token) return@launch
            when (result) {
                is CallResult.Success -> _state.update { it.copy(searching = false, searched = true, results = result.value.items) }
                is CallResult.Failure -> _state.update { it.copy(searching = false, error = result.error, results = emptyList()) }
                CallResult.Stale -> Unit
            }
        }
    }

    /** [bangumiId] 0 removes the binding. */
    fun bind(bangumiId: Long) {
        val owner = sessions.connected ?: return
        if (_state.value.binding != null) return
        _state.update { it.copy(binding = bangumiId, error = null) }
        viewModelScope.launch {
            val result = sessions.call(owner, io) { bindMetadata(animeId, bangumiId) }
            when (result) {
                is CallResult.Success -> _state.update { it.copy(binding = null, done = true) }
                is CallResult.Failure -> _state.update { it.copy(binding = null, error = result.error) }
                CallResult.Stale -> Unit
            }
        }
    }

    private companion object {
        const val MIN_QUERY = 2
    }
}

/** Drops [tags], bracketed notes and anything after " / ", matching the web bind dialog. */
internal fun cleanTitle(raw: String): String {
    val head = raw
        .replace(Regex("[\\[【][^\\]】]*[\\]】]"), " ")
        .replace(Regex("[（(][^）)]*[）)]"), " ")
        .split(Regex("\\s+[/|]\\s+"))
        .first()
    return head.replace(Regex("\\s+"), " ").trim()
}
