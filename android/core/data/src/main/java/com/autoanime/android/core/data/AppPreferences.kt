package com.autoanime.android.core.data

import android.content.Context
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.floatPreferencesKey
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

    /** Playback speed applied to every player until changed; volumes stay under system control. */
    val playbackRate: Flow<Float> = context.appPreferences.data.map { it[PLAYBACK_RATE] ?: 1f }

    suspend fun setPlaybackRate(rate: Float) {
        context.appPreferences.edit { it[PLAYBACK_RATE] = rate }
    }

    private companion object {
        val DYNAMIC_COLOR = booleanPreferencesKey("dynamic_color")
        val PLAYBACK_RATE = floatPreferencesKey("playback_rate")
    }
}
