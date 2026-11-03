package com.autoanime.android.feature.player

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class PlayerQueueTest {
    private val queue = PlayerQueue(
        listOf(
            PlayerQueue.Entry(1, "第 1 集"),
            PlayerQueue.Entry(2, "第 2 集"),
            PlayerQueue.Entry(3, "第 3 集"),
        ),
        1,
    )

    @Test fun `start index is clamped into the queue`() {
        assertEquals(1, queue.index)
        assertEquals(0, PlayerQueue(emptyList(), 4).index)
    }

    @Test fun `empty queue exposes no entries`() {
        val empty = PlayerQueue(emptyList(), 0)
        assertFalse(empty.hasNext)
        assertFalse(empty.hasPrev)
        assertNull(empty.peekNext())
        assertNull(empty.next())
    }

    @Test fun `next and prev move within bounds`() {
        assertTrue(queue.hasPrev)
        assertTrue(queue.hasNext)
        assertEquals(3L, queue.next()?.id)
        assertEquals(2, queue.index)
        assertFalse(queue.hasNext)
        assertNull(queue.next())
        assertEquals(2L, queue.prev()?.id)
        assertEquals(1, queue.index)
        assertEquals(1L, queue.prev()?.id)
        assertFalse(queue.hasPrev)
        assertNull(queue.prev())
    }

    @Test fun `peek does not move the index`() {
        assertEquals(3L, queue.peekNext()?.id)
        assertEquals(1L, queue.peekPrev()?.id)
        assertEquals(1, queue.index)
    }
}
