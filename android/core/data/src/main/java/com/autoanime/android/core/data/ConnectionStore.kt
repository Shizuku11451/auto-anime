package com.autoanime.android.core.data

import android.content.Context
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.first
import javax.inject.Inject
import javax.inject.Singleton

/** The last server the user connected to. Never contains a password. */
data class SavedConnection(val url: String, val allowHttp: Boolean, val account: String)

interface ConnectionStore {
    suspend fun load(): SavedConnection?
    suspend fun save(connection: SavedConnection)
    suspend fun clear()
}

// File and key names match the Views prototype so existing installs keep their saved server.
private val Context.connectionPreferences by preferencesDataStore(name = "connection")

@Singleton
class DataStoreConnectionStore @Inject constructor(
    @ApplicationContext private val context: Context,
) : ConnectionStore {
    override suspend fun load(): SavedConnection? {
        val values = context.connectionPreferences.data.first()
        val url = values[SERVER_URL] ?: return null
        return SavedConnection(url, values[ALLOW_HTTP] ?: false, values[ACCOUNT].orEmpty())
    }

    override suspend fun save(connection: SavedConnection) {
        context.connectionPreferences.edit {
            it[SERVER_URL] = connection.url
            it[ALLOW_HTTP] = connection.allowHttp
            it[ACCOUNT] = connection.account
        }
    }

    override suspend fun clear() {
        context.connectionPreferences.edit { it.clear() }
    }

    private companion object {
        val SERVER_URL = stringPreferencesKey("server_url")
        val ALLOW_HTTP = booleanPreferencesKey("allow_http")
        val ACCOUNT = stringPreferencesKey("account")
    }
}
