package com.autoanime.android.core.database

import androidx.room.Database
import androidx.room.RoomDatabase

@Database(entities = [WatchProgressEntity::class], version = 1, exportSchema = false)
abstract class AutoAnimeDatabase : RoomDatabase() {
    abstract fun watchProgressDao(): WatchProgressDao
}
