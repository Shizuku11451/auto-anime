package com.autoanime.android.core.data

import android.content.Context
import coil3.ImageLoader
import coil3.memory.MemoryCache
import coil3.network.okhttp.OkHttpNetworkFetcherFactory
import com.autoanime.android.core.network.AnimeApi
import com.autoanime.android.core.network.CookieStorage
import com.autoanime.android.core.network.EncryptedCookieStorage
import dagger.Binds
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import javax.inject.Qualifier
import javax.inject.Singleton

@Qualifier @Retention(AnnotationRetention.BINARY) annotation class IoDispatcher
@Qualifier @Retention(AnnotationRetention.BINARY) annotation class ApplicationScope

@Module
@InstallIn(SingletonComponent::class)
object DataModule {
    @Provides @Singleton
    fun cookieStorage(@ApplicationContext context: Context): CookieStorage = EncryptedCookieStorage(context)

    @Provides
    fun apiFactory(): ApiFactory = ApiFactory { server, cookies -> AnimeApi(server.newHttpClient(cookies), server) }

    @Provides
    fun coverImageLoaderFactory(@ApplicationContext context: Context): CoverImageLoaderFactory =
        CoverImageLoaderFactory { client ->
            ImageLoader.Builder(context)
                .components { add(OkHttpNetworkFetcherFactory(callFactory = { client })) }
                // Covers require the session cookie; nothing is written to disk.
                .diskCache(null)
                .memoryCache { MemoryCache.Builder().maxSizePercent(context, 0.08).build() }
                .build()
        }

    @Provides @Singleton @ApplicationScope
    fun applicationScope(): CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    @Provides @IoDispatcher
    fun ioDispatcher(): CoroutineDispatcher = Dispatchers.IO
}

@Module
@InstallIn(SingletonComponent::class)
abstract class DataBindings {
    @Binds
    abstract fun connectionStore(store: DataStoreConnectionStore): ConnectionStore
}
