package com.autoanime.android.core.data

import android.content.Context
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.preferencesDataStore
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import javax.inject.Inject
import javax.inject.Singleton

private val Context.appPreferences by preferencesDataStore(name = "preferences")

/** Client-only display preferences; nothing here is sent to the server. */
@Singleton
class AppPreferences @Inject constructor(@ApplicationContext private val context: Context) {
    val dynamicColor: Flow<Boolean> = context.appPreferences.data.map { it[DYNAMIC_COLOR] ?: false }

    suspend fun setDynamicColor(enabled: Boolean) {
        context.appPreferences.edit { it[DYNAMIC_COLOR] = enabled }
    }

    private companion object {
        val DYNAMIC_COLOR = booleanPreferencesKey("dynamic_color")
    }
}
