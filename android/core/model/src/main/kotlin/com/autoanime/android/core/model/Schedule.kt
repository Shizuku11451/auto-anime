package com.autoanime.android.core.model

/**
 * Library schedule buckets, numbered like the server and `web/src/components/library/schedule.ts`:
 * 0 no known update day, 1-7 Monday to Sunday, 8 finished airing.
 */
object ScheduleDay {
    const val UNKNOWN = 0
    const val FINISHED = 8
    val WEEKDAYS = 1..7
}
