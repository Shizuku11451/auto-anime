package com.autoanime.android

import android.app.Application
import com.autoanime.android.core.EncryptedCookieStorage
import okhttp3.OkHttpClient
import coil3.ImageLoader
import coil3.network.okhttp.OkHttpNetworkFetcherFactory
import java.util.concurrent.atomic.AtomicLong
import java.util.concurrent.TimeUnit
import coil3.memory.MemoryCache
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.first

private val android.content.Context.preferences by preferencesDataStore(name = "connection")

class AutoAnimeApp : Application(), SessionConnection {
    override val generation = AtomicLong(0)
    override val storage by lazy { EncryptedCookieStorage(this) }
    override suspend fun saveConnection(url: String, allowHttp: Boolean, account: String, expectedGeneration: Long) {
        if (generation.get() != expectedGeneration) return
        preferences.edit { if (generation.get() == expectedGeneration) { it[SERVER_URL] = url; it[ALLOW_HTTP] = allowHttp; it[ACCOUNT] = account } }
    }
    suspend fun savedConnection(): SavedConnection? {
        val values = preferences.data.first()
        val url = values[SERVER_URL] ?: return null
        return SavedConnection(url, values[ALLOW_HTTP] ?: false, values[ACCOUNT].orEmpty())
    }
    override suspend fun clearSavedConnection(expectedGeneration: Long) {
        if (generation.get() == expectedGeneration) preferences.edit { if (generation.get() == expectedGeneration) it.clear() }
    }
    override var http: OkHttpClient? = null
    override var streamHttp: OkHttpClient? = null
    private var coverClient: OkHttpClient? = null
    private val imageLoaderLazy = lazy {
        ImageLoader.Builder(this).components {
            add(OkHttpNetworkFetcherFactory(callFactory = { requireNotNull(coverClient) }))
        }.diskCache(null).memoryCache { MemoryCache.Builder().maxSizePercent(this@AutoAnimeApp, 0.08).build() }.build()
    }
    val imageLoader by imageLoaderLazy

    override fun connectCovers(client: OkHttpClient) {
        coverClient?.dispatcher?.cancelAll()
        coverClient = client.newBuilder().callTimeout(15, TimeUnit.SECONDS).build()
        if (imageLoaderLazy.isInitialized()) imageLoader.memoryCache?.clear()
    }

    override fun clearSession() {
        generation.incrementAndGet()
        http?.dispatcher?.cancelAll()
        http?.connectionPool?.evictAll()
        http = null
        coverClient?.dispatcher?.cancelAll()
        coverClient?.connectionPool?.evictAll()
        coverClient = null
        streamHttp?.dispatcher?.cancelAll()
        streamHttp?.connectionPool?.evictAll()
        streamHttp = null
        if (imageLoaderLazy.isInitialized()) imageLoader.memoryCache?.clear()
    }
    companion object {
        private val SERVER_URL = stringPreferencesKey("server_url")
        private val ALLOW_HTTP = booleanPreferencesKey("allow_http")
        private val ACCOUNT = stringPreferencesKey("account")
    }
}

interface SessionConnection {
    val generation: AtomicLong
    val storage: com.autoanime.android.core.CookieStorage
    var http: OkHttpClient?
    var streamHttp: OkHttpClient?
    fun clearSession()
    fun connectCovers(client: OkHttpClient)
    suspend fun saveConnection(url: String, allowHttp: Boolean, account: String, expectedGeneration: Long)
    suspend fun clearSavedConnection(expectedGeneration: Long)
}

data class SavedConnection(val url: String, val allowHttp: Boolean, val account: String)
