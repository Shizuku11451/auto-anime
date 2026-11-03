package com.autoanime.android.core.database

import android.content.Context
import androidx.room.Room
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

@Module
@InstallIn(SingletonComponent::class)
object DatabaseModule {
    @Provides
    @Singleton
    fun database(@ApplicationContext context: Context): AutoAnimeDatabase =
        Room.databaseBuilder(context, AutoAnimeDatabase::class.java, "auto-anime.db").build()

    @Provides
    fun watchProgressDao(database: AutoAnimeDatabase): WatchProgressDao = database.watchProgressDao()
}
