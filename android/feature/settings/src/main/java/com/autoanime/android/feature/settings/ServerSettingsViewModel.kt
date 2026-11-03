package com.autoanime.android.feature.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.autoanime.android.core.data.CallResult
import com.autoanime.android.core.data.IoDispatcher
import com.autoanime.android.core.data.SessionRepository
import com.autoanime.android.core.data.call
import com.autoanime.android.core.data.connected
import com.autoanime.android.core.model.AppError
import com.autoanime.android.core.model.MAX_CONCURRENT_DOWNLOADS
import com.autoanime.android.core.model.MAX_POLL_MINUTES
import com.autoanime.android.core.model.MIN_POLL_MINUTES
import com.autoanime.android.core.model.ServerSettings
import com.autoanime.android.core.model.SettingsInput
import com.autoanime.android.core.model.SettingsSaveResult
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

/** Numbers are text so a half-typed value is not lost. */
data class ServerForm(
    val pollMinutes: String = "",
    val maxConcurrent: String = "",
    val keepSourceFiles: Boolean = true,
    val autoCleanup: Boolean = false,
    val libraryDir: String = "",
)

enum class ServerField { PollMinutes, MaxConcurrent }

data class ServerSettingsUiState(
    val loading: Boolean = true,
    val loadError: AppError? = null,
    /** As last read from the server; the form is compared against it to send only changes. */
    val stored: ServerSettings? = null,
    val form: ServerForm = ServerForm(),
    val invalid: ServerField? = null,
    val saving: Boolean = false,
    val saveError: AppError? = null,
    val saved: SettingsSaveResult? = null,
) {
    val changed: Boolean get() = stored != null && stored.toForm() != form
}

/**
 * The server settings that make sense from a phone. Downloader connections, credentials and the
 * proxy stay in the web client; the proxy and download mode are shown read-only.
 */
@HiltViewModel
class ServerSettingsViewModel @Inject constructor(
    private val sessions: SessionRepository,
    @IoDispatcher private val io: CoroutineDispatcher,
) : ViewModel() {
    private val _state = MutableStateFlow(ServerSettingsUiState())
    val state: StateFlow<ServerSettingsUiState> = _state.asStateFlow()

    init {
        load()
    }

    fun load() {
        val owner = sessions.connected ?: return
        _state.update { it.copy(loading = true, loadError = null) }
        viewModelScope.launch {
            when (val result = sessions.call(owner, io) { settings() }) {
                is CallResult.Success -> _state.update {
                    it.copy(loading = false, stored = result.value, form = result.value.toForm(), saveError = null)
                }
                is CallResult.Failure -> _state.update { it.copy(loading = false, loadError = result.error) }
                CallResult.Stale -> Unit
            }
        }
    }

    fun update(transform: (ServerForm) -> ServerForm) = _state.update {
        it.copy(form = transform(it.form), invalid = null, saveError = null, saved = null)
    }

    fun save() {
        val owner = sessions.connected ?: return
        val current = _state.value
        val stored = current.stored ?: return
        if (current.saving) return
        val form = current.form
        val poll = form.pollMinutes.trim().toIntOrNull()?.takeIf { it in MIN_POLL_MINUTES..MAX_POLL_MINUTES }
        val limit = form.maxConcurrent.trim().toIntOrNull()?.takeIf { it in 1..MAX_CONCURRENT_DOWNLOADS }
        when {
            poll == null -> return _state.update { it.copy(invalid = ServerField.PollMinutes) }
            limit == null -> return _state.update { it.copy(invalid = ServerField.MaxConcurrent) }
        }
        val input = SettingsInput(
            poll_minutes = poll.takeIf { it != stored.poll_minutes },
            max_concurrent_downloads = limit.takeIf { it != stored.max_concurrent_downloads },
            keep_source_files = form.keepSourceFiles.takeIf { it != stored.keep_source_files },
            auto_cleanup_empty_anime = form.autoCleanup.takeIf { it != stored.auto_cleanup_empty_anime },
            library_dir = form.libraryDir.trim().takeIf { it != stored.library_dir },
        )
        if (input == SettingsInput()) return
        _state.update { it.copy(saving = true, saveError = null) }
        viewModelScope.launch {
            when (val result = sessions.call(owner, io) { saveSettings(input) }) {
                is CallResult.Success -> {
                    _state.update { it.copy(saving = false, saved = result.value) }
                    load()
                }
                is CallResult.Failure -> _state.update { it.copy(saving = false, saveError = result.error) }
                CallResult.Stale -> Unit
            }
        }
    }
}

private fun ServerSettings.toForm() = ServerForm(
    pollMinutes = poll_minutes.toString(),
    maxConcurrent = max_concurrent_downloads.toString(),
    keepSourceFiles = keep_source_files,
    autoCleanup = auto_cleanup_empty_anime,
    libraryDir = library_dir,
)
