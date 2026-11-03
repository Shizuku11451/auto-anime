package com.autoanime.android

import com.autoanime.android.core.data.ConnectionStore
import com.autoanime.android.core.data.SessionRepository
import com.autoanime.android.core.network.CookieStorage
import dagger.hilt.EntryPoint
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent

/**
 * Debug-only access to app singletons, so instrumented tests can check for a disposable install
 * and reset the session they created. Not compiled into release builds.
 */
@EntryPoint
@InstallIn(SingletonComponent::class)
interface DebugEntryPoint {
    fun sessions(): SessionRepository
    fun connectionStore(): ConnectionStore
    fun cookieStorage(): CookieStorage
}
