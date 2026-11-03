package com.autoanime.android

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.autoanime.android.core.data.AppPreferences
import com.autoanime.android.core.data.SessionRepository
import com.autoanime.android.core.data.SessionState
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.stateIn
import javax.inject.Inject

@HiltViewModel
class AppViewModel @Inject constructor(
    sessions: SessionRepository,
    preferences: AppPreferences,
) : ViewModel() {
    val session: StateFlow<SessionState> = sessions.state
    val dynamicColor: StateFlow<Boolean> =
        preferences.dynamicColor.stateIn(viewModelScope, SharingStarted.Eagerly, false)

    init {
        sessions.restore()
    }
}
