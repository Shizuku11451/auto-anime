package com.autoanime.android.core.model

import kotlinx.serialization.Serializable
import java.net.URI
import java.net.URLDecoder
import java.net.URLEncoder

/** One subscription from `/api/feeds`. [url] may carry a private token, so it is never shown in full. */
@Serializable
data class Feed(
    val id: Long,
    val name: String,
    val url: String = "",
    val include_words: String = "",
    val exclude_words: String = "",
    val auto_download: Boolean = false,
    val enabled: Boolean = true,
    val last_sync: String = "",
    val last_error: String = "",
    val target_title: String = "",
    val storage_dir: String = "",
    val season_filter: Int = SEASON_ANY,
    val allow_unknown_season: Boolean = false,
    val episode_offset: Int? = null,
    val completion_note: String = "",
)

/**
 * Body for creating (`POST /api/feeds`) or editing (`PUT /api/feeds/{id}`) a subscription. The
 * server always overwrites the first four fields; a null optional field keeps the stored value.
 * The address can only be set on creation.
 */
@Serializable
data class FeedInput(
    val name: String,
    val include_words: String,
    val exclude_words: String,
    val auto_download: Boolean,
    val url: String? = null,
    val enabled: Boolean? = null,
    val target_title: String? = null,
    val season_filter: Int? = null,
    val allow_unknown_season: Boolean? = null,
    val episode_offset: Int? = null,
)

@Serializable data class FeedSaveResult(val id: Long = 0, val storage_warning: String = "")

@Serializable
data class PreviewInput(
    val url: String,
    val include_words: String,
    val exclude_words: String,
    val season_filter: Int,
    val allow_unknown_season: Boolean,
)

/** Release links in the preview are not decoded; only titles are shown. */
@Serializable data class PreviewEntry(val title: String = "")

@Serializable
data class PreviewResult(
    val total: Int = 0,
    val count: Int = 0,
    val items: List<PreviewEntry> = emptyList(),
    val warning: String = "",
    val limited: Boolean = false,
)

@Serializable data class SyncResult(val added: Int = 0)

/** Season filter values: every season, only releases without a season mark, or 1-999. */
const val SEASON_ANY = 0
const val SEASON_UNMARKED = -1

val Feed.isAggregate: Boolean get() = url.startsWith(AGGREGATE_PREFIX)

/**
 * The scheme and host of an RSS address, without path, query or user info, which can hold a
 * private token. Aggregate subscriptions return null; the UI labels them separately.
 */
fun Feed.displayHost(): String? {
    if (isAggregate) return null
    return runCatching {
        val uri = URI(url)
        val host = uri.host ?: return null
        val port = if (uri.port > 0) ":${uri.port}" else ""
        "${uri.scheme}://$host$port"
    }.getOrNull()
}

/**
 * The version rules an aggregate subscription selects, mirroring `web/src/utils/aggregateSubscription.ts`.
 * Returns null when the resource cannot back a subscription.
 */
fun AggregateResource.subscriptionRules(subjectId: Long): Map<String, String>? {
    if (subjectId <= 0 || subjectId > Int.MAX_VALUE || season !in 0..999 || anime_title.isBlank() || !MAGNET.matches(magnet)) return null
    return sortedMapOf(
        "group" to fansub_group,
        "language" to (LANGUAGE.find(title)?.value ?: ""),
        "quality" to quality,
        "season" to season.toString(),
        "title" to anime_title,
    )
}

fun aggregateUrl(subjectId: Long, rules: Map<String, String>): String =
    "aggregate://subject/$subjectId?" + rules.toSortedMap().entries.joinToString("&") { (k, v) -> "${encode(k)}=${encode(v)}" }

/** Compares decoded rules, so differences in how the server and client percent-encode do not matter. */
fun List<Feed>.findAggregate(subjectId: Long, rules: Map<String, String>): Feed? = firstOrNull { feed ->
    val base = feed.url.substringBefore('?')
    base == "aggregate://subject/$subjectId" && decodeQuery(feed.url.substringAfter('?', "")) == rules.toSortedMap()
}

private fun decodeQuery(query: String): Map<String, String> = query.split('&').filter { it.isNotEmpty() }.associate {
    URLDecoder.decode(it.substringBefore('='), "UTF-8") to URLDecoder.decode(it.substringAfter('=', ""), "UTF-8")
}.toSortedMap()

// The Charset overloads of URLEncoder/URLDecoder need Android 13; the client supports Android 8.
private fun encode(value: String) = URLEncoder.encode(value, "UTF-8")

private const val AGGREGATE_PREFIX = "aggregate:"
private val MAGNET = Regex("^magnet:\\?xt=urn:btih:[a-f0-9]{40}$", RegexOption.IGNORE_CASE)
private val LANGUAGE = Regex(
    "简繁|簡繁|简日|簡日|繁日|简中|簡中|繁中|简体|簡體|繁体|繁體|\\bCHS\\b|\\bCHT\\b|\\bGB\\b|\\bBIG5\\b",
    RegexOption.IGNORE_CASE,
)
