package com.autoanime.android.core.model

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class FeedsTest {
    @Test fun displayHostDropsPathQueryAndCredentials() {
        val feed = Feed(1, "x", url = "https://user:secret@rss.example:8443/rss/private?token=abc#frag")
        assertEquals("https://rss.example:8443", feed.displayHost())
        assertEquals("https://mikan.example", Feed(2, "y", url = "https://mikan.example/RSS/MyBangumi?token=xyz").displayHost())
        assertNull(Feed(3, "z", url = "aggregate://subject/1?title=a").displayHost())
        assertNull(Feed(4, "w", url = "not a url").displayHost())
    }

    @Test fun subscriptionRulesMirrorTheWebClient() {
        val resource = resource(title = "[Group] 葬送的芙莉莲 - 07 [1080p][简日]", season = 1)
        assertEquals(
            mapOf("group" to "Group", "language" to "简日", "quality" to "1080p", "season" to "1", "title" to "葬送的芙莉莲"),
            resource.subscriptionRules(425),
        )
        assertEquals("", resource(title = "[Group] Title [CHT2]").subscriptionRules(1)!!["language"])
        assertEquals("CHT", resource(title = "[Group] Title [CHT]").subscriptionRules(1)!!["language"])
    }

    @Test fun invalidResourcesCannotBackASubscription() {
        assertNull(resource(magnet = "magnet:?xt=urn:btih:short").subscriptionRules(1))
        assertNull(resource(magnet = "https://example/file.torrent").subscriptionRules(1))
        assertNull(resource(animeTitle = " ").subscriptionRules(1))
        assertNull(resource(season = 1000).subscriptionRules(1))
        assertNull(resource().subscriptionRules(0))
        assertNull(resource().subscriptionRules(Int.MAX_VALUE.toLong() + 1))
    }

    @Test fun existingSubscriptionMatchesRegardlessOfEncoding() {
        val rules = resource(title = "[A B] 名 & 字 [1080p]", group = "A B").subscriptionRules(9)!!
        val url = aggregateUrl(9, rules)
        assertEquals(url, aggregateUrl(9, rules.toList().reversed().toMap()))
        // The server re-encodes with Go's url.Values; spaces may arrive as %20 instead of +.
        val serverForm = url.replace("+", "%20")
        val feeds = listOf(Feed(1, "other", url = "aggregate://subject/10?" + url.substringAfter('?')), Feed(2, "mine", url = serverForm))
        assertEquals(2L, feeds.findAggregate(9, rules)?.id)
        assertNull(feeds.findAggregate(9, rules + ("quality" to "720p")))
    }

    private fun resource(
        title: String = "[Group] Title - 01 [1080p]",
        animeTitle: String = "葬送的芙莉莲",
        group: String = "Group",
        season: Int = 1,
        magnet: String = "magnet:?xt=urn:btih:" + "a".repeat(40),
    ) = AggregateResource(title = title, anime_title = animeTitle, fansub_group = group, quality = "1080p", season = season, magnet = magnet)
}
