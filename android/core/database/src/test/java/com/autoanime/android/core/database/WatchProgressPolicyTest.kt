package com.autoanime.android.core.database

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class WatchProgressPolicyTest {
    @Test
    fun `positions in the first ten seconds are not kept`() {
        assertNull(WatchProgressPolicy.positionToKeep(0, 600_000))
        assertNull(WatchProgressPolicy.positionToKeep(9_999, 600_000))
        assertEquals(10_000L, WatchProgressPolicy.positionToKeep(10_000, 600_000))
    }

    @Test
    fun `positions within the end margin are treated as watched`() {
        assertNull(WatchProgressPolicy.positionToKeep(585_000, 600_000))
        assertNull(WatchProgressPolicy.positionToKeep(600_000, 600_000))
        assertEquals(584_999L, WatchProgressPolicy.positionToKeep(584_999, 600_000))
    }

    @Test
    fun `unknown duration only applies the start rule`() {
        assertNull(WatchProgressPolicy.positionToKeep(5_000, 0))
        assertEquals(30_000L, WatchProgressPolicy.positionToKeep(30_000, 0))
    }

    @Test
    fun `resume follows the keep rules`() {
        assertNull(WatchProgressPolicy.resumePosition(9_999, 600_000))
        assertNull(WatchProgressPolicy.resumePosition(585_000, 600_000))
        assertEquals(300_000L, WatchProgressPolicy.resumePosition(300_000, 600_000))
    }

    @Test
    fun `a record beyond the current duration starts over`() {
        assertNull(WatchProgressPolicy.resumePosition(700_000, 600_000))
        assertNull(WatchProgressPolicy.resumePosition(600_001, 600_000))
        // The length is still unknown while metadata loads; the record is trusted.
        assertEquals(300_000L, WatchProgressPolicy.resumePosition(300_000, 0))
    }
}
