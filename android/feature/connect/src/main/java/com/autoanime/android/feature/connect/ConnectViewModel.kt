package com.autoanime.android.feature.connect

import androidx.lifecycle.ViewModel
import com.autoanime.android.core.data.SessionRepository
import com.autoanime.android.core.data.SessionState
import com.autoanime.android.core.network.InvalidServerUrl
import com.autoanime.android.core.network.ServerConnection
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.StateFlow
import javax.inject.Inject

@HiltViewModel
class ConnectViewModel @Inject constructor(private val sessions: SessionRepository) : ViewModel() {
    val state: StateFlow<SessionState> = sessions.state

    /** Starts connecting, or returns why the URL was rejected without touching the network. */
    fun connect(url: String, allowHttp: Boolean, account: String, password: String): InvalidServerUrl.Reason? {
        val server = try {
            ServerConnection.parse(url, allowHttp)
        } catch (e: InvalidServerUrl) {
            return e.reason
        }
        sessions.connect(server, account.trim(), password)
        return null
    }
}
