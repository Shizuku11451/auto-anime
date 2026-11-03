package com.autoanime.android.core.database

import androidx.room.Entity
import androidx.room.PrimaryKey

/** Last playback position of one saved release, kept on the device for resume. */
@Entity(tableName = "watch_progress")
data class WatchProgressEntity(
    @PrimaryKey val item_id: Long,
    val anime_id: Long,
    val position_ms: Long,
    val duration_ms: Long,
    val updated_at: Long,
)
