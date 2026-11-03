package com.autoanime.android.core.model

import kotlinx.serialization.Serializable

@Serializable
data class DiscoverSubject(
    val id: Long,
    val name: String = "",
    val name_cn: String = "",
    val cover: String = "",
    val summary: String = "",
    val date: String = "",
    val rating: Double = 0.0,
)

/** [resources_enabled] is false when the server has no aggregate search service configured. */
@Serializable
data class DiscoverResult(
    val keyword: String = "",
    val items: List<DiscoverSubject> = emptyList(),
    val resources_enabled: Boolean = false,
)

/** A release found by the aggregate search service; the magnet is only used to validate it. */
@Serializable
data class AggregateResource(
    val title: String = "",
    val anime_title: String = "",
    val fansub_group: String = "",
    val episode: String = "",
    val quality: String = "",
    val season: Int = 0,
    val size: Long = 0,
    val published: String = "",
    val magnet: String = "",
    val sources: List<String> = emptyList(),
    val needs_review: Boolean = false,
)

/** [warnings] report sources that failed; the remaining results are still usable. */
@Serializable
data class AggregateResult(
    val subject_id: Long = 0,
    val items: List<AggregateResource> = emptyList(),
    val warnings: List<String> = emptyList(),
)

val DiscoverSubject.displayName: String get() = name_cn.ifBlank { name }
