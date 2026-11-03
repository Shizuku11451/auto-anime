package com.autoanime.android.core.database

/**
 * Thresholds for keeping and restoring a playback position, mirroring the web player: a position
 * is kept only past the first ten seconds and more than fifteen seconds before the end, and is
 * restored under the same rules so a nearly finished file starts over instead of jumping to the end.
 */
object WatchProgressPolicy {
    /** Positions before this are treated as "just started" and not kept. */
    const val MIN_POSITION_MS = 10_000L

    /** Positions within this distance of the end are treated as "watched" and not kept. */
    const val END_MARGIN_MS = 15_000L

    /**
     * The position to persist for [durationMs], or null when it must be dropped (near the start,
     * or close enough to the end that the next play should start over). A non-positive duration
     * means the length is unknown and only the start rule applies.
     */
    fun positionToKeep(positionMs: Long, durationMs: Long): Long? = when {
        positionMs < MIN_POSITION_MS -> null
        durationMs > 0 && positionMs >= durationMs - END_MARGIN_MS -> null
        else -> positionMs
    }

    /**
     * The position to resume from, or null to start over: the record must clear the same rules as
     * [positionToKeep], and a record beyond the current length means the file behind the item changed.
     */
    fun resumePosition(savedMs: Long, durationMs: Long): Long? = when {
        savedMs < MIN_POSITION_MS -> null
        durationMs > 0 && savedMs >= durationMs - END_MARGIN_MS -> null
        durationMs > 0 && savedMs > durationMs -> null
        else -> savedMs
    }
}
