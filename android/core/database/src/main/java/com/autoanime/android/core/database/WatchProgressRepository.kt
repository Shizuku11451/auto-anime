package com.autoanime.android.core.database

import javax.inject.Inject
import javax.inject.Singleton

/** Reads and writes the resume position of saved releases on this device. */
@Singleton
class WatchProgressRepository @Inject constructor(private val dao: WatchProgressDao) {
    /** The stored position for [itemId], already cleared of records that would not resume. */
    suspend fun savedPosition(itemId: Long, durationMs: Long = 0): Long? {
        val saved = dao.get(itemId)?.position_ms ?: return null
        return WatchProgressPolicy.resumePosition(saved, durationMs)
    }

    /** Keeps [positionMs] for [itemId], or drops the record when it must not resume. */
    suspend fun update(itemId: Long, animeId: Long, positionMs: Long, durationMs: Long) {
        val keep = WatchProgressPolicy.positionToKeep(positionMs, durationMs)
        if (keep == null) {
            dao.delete(itemId)
        } else {
            dao.upsert(WatchProgressEntity(itemId, animeId, keep, durationMs, System.currentTimeMillis()))
        }
    }

    suspend fun clear(itemId: Long) = dao.delete(itemId)
}
