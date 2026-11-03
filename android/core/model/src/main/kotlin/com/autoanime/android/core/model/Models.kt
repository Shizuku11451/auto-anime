package com.autoanime.android.core.model

import kotlinx.serialization.Serializable

/** [day_counts] is present only when requested and the server supports schedule filtering. */
@Serializable data class AnimePage(val items: List<Anime>, val next_before: Long, val day_counts: List<Int>? = null)
@Serializable data class Anime(
    val id: Long,
    val title: String,
    val season: Int = 0,
    val cover: String = "",
    val meta_name: String = "",
    val meta_name_cn: String = "",
    val summary: String = "",
    val update_weekday: Int = 0,
    val broadcast_weekday: Int = 0,
    val broadcast_end: String = "",
)
@Serializable data class LoginResult(val ok: Boolean)
@Serializable
data class AnimeDetail(
    val id: Long,
    val title: String,
    val cover: String = "",
    val summary: String = "",
    val seasons: List<Season> = emptyList(),
    val search_name: String = "",
    val bangumi_id: Long = 0,
    val season: Int = 0,
    val storage_dir: String = "",
    val include_rules: String = "",
    val exclude_rules: String = "",
    val update_weekday: Int = 0,
    val meta_name: String = "",
    val meta_name_cn: String = "",
    val meta_source: String = "",
)
@Serializable data class Season(val season: Int, val episodes: List<Episode> = emptyList(), val loose: List<MediaItem> = emptyList())
@Serializable data class Episode(val episode: String, val items: List<MediaItem> = emptyList())
@Serializable
data class MediaItem(
    val id: Long,
    val title: String,
    val status: String,
    val episode: String = "",
    val quality: String = "",
    val file_path: String = "",
    val anime_id: Long = 0,
    /** Season set on the item itself; 0 follows the anime. */
    val season: Int = 0,
    /** Season parsed from the release title, used when neither the item nor the anime sets one. */
    val season_auto: Int = 0,
    val fansub_group: String = "",
)
@Serializable data class SetupState(val required: Boolean, val download_mode: String? = null)

/** Status string the server assigns once a file is archived; only those items are playable. */
const val STATUS_SAVED = "已保存"

val MediaItem.isSaved: Boolean get() = status == STATUS_SAVED

fun AnimeDetail.allItems(): List<MediaItem> = seasons.flatMap { season -> season.episodes.flatMap { it.items } + season.loose }

fun AnimeDetail.savedItems(): List<MediaItem> = allItems().filter { it.isSaved }
