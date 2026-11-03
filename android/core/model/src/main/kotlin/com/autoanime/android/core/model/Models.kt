package com.autoanime.android.core.model

import kotlinx.serialization.Serializable

@Serializable data class AnimePage(val items: List<Anime>, val next_before: Long)
@Serializable data class Anime(val id: Long, val title: String, val season: Int = 0, val cover: String = "", val meta_name: String = "", val meta_name_cn: String = "", val summary: String = "")
@Serializable data class LoginResult(val ok: Boolean)
@Serializable data class AnimeDetail(val id: Long, val title: String, val cover: String = "", val summary: String = "", val seasons: List<Season> = emptyList())
@Serializable data class Season(val season: Int, val episodes: List<Episode> = emptyList(), val loose: List<MediaItem> = emptyList())
@Serializable data class Episode(val episode: String, val items: List<MediaItem> = emptyList())
@Serializable data class MediaItem(val id: Long, val title: String, val status: String, val episode: String = "", val quality: String = "", val file_path: String = "")
@Serializable data class SetupState(val required: Boolean, val download_mode: String? = null)

/** Status string the server assigns once a file is archived; only those items are playable. */
const val STATUS_SAVED = "已保存"

val MediaItem.isSaved: Boolean get() = status == STATUS_SAVED

fun AnimeDetail.savedItems(): List<MediaItem> = seasons.flatMap { season -> season.episodes.flatMap { it.items } + season.loose }.filter { it.isSaved }
