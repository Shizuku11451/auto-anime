package com.autoanime.android.core.database

import androidx.room.Dao
import androidx.room.Query
import androidx.room.Upsert

@Dao
interface WatchProgressDao {
    @Query("SELECT * FROM watch_progress WHERE item_id = :itemId")
    suspend fun get(itemId: Long): WatchProgressEntity?

    @Upsert
    suspend fun upsert(entity: WatchProgressEntity)

    @Query("DELETE FROM watch_progress WHERE item_id = :itemId")
    suspend fun delete(itemId: Long)
}
