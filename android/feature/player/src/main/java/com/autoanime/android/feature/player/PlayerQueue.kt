package com.autoanime.android.feature.player

/**
 * The ordered playable releases handed to one player screen. Advancing mutates [index]; the
 * player screen stays mounted while the media source is swapped to the new entry.
 */
class PlayerQueue(entries: List<Entry>, startIndex: Int) {
    data class Entry(val id: Long, val label: String, val animeId: Long = 0)

    private val items = entries
    var index: Int = startIndex.coerceIn(0, (items.size - 1).coerceAtLeast(0))
        private set

    val current: Entry get() = items[index]
    val hasNext: Boolean get() = index < items.lastIndex
    val hasPrev: Boolean get() = index > 0

    /** Moves to the next entry and returns it, or null when this is the end of the queue. */
    fun next(): Entry? = items.getOrNull(index + 1)?.also { index++ }

    /** Moves to the previous entry and returns it, or null at the start of the queue. */
    fun prev(): Entry? = items.getOrNull(index - 1)?.also { index-- }

    fun peekNext(): Entry? = items.getOrNull(index + 1)
    fun peekPrev(): Entry? = items.getOrNull(index - 1)
}
