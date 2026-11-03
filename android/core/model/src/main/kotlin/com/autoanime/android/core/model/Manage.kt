package com.autoanime.android.core.model

import kotlinx.serialization.Serializable

/** Full body for `PUT /api/animes/{id}`; fields the client does not edit are sent back unchanged. */
@Serializable
data class AnimeUpdateInput(
    val title: String,
    val search_name: String,
    val season: Int,
    val update_weekday: Int,
    val storage_dir: String,
    val include_rules: String,
    val exclude_rules: String,
)

@Serializable data class AnimeSaveResult(val storage_warning: String = "")
@Serializable data class ApplyRulesResult(val matched: Int = 0)
@Serializable data class RematchResult(val queued: Boolean = false)

@Serializable data class MetaCandidate(val id: Long, val name: String = "", val name_cn: String = "")
@Serializable data class MetaSearchResult(val items: List<MetaCandidate> = emptyList())
@Serializable data class BindInput(val bangumi_id: Long)

/**
 * `PATCH /api/items/{id}`: move a release to another anime (by ID, or by title, which the server
 * matches or creates), season (0 follows the anime) and episode. [rearchive] also moves saved media.
 */
@Serializable
data class CorrectInput(
    val season: Int,
    val episode: String,
    val rearchive: Boolean,
    val anime_id: Long? = null,
    val anime_title: String? = null,
)

@Serializable data class CorrectResult(val moved: Boolean = false, val cleanup_warning: String = "")

/** The server settings this client shows; secrets are only reported as "set". */
@Serializable
data class ServerSettings(
    val max_concurrent_downloads: Int = 1,
    val auto_cleanup_empty_anime: Boolean = false,
    val keep_source_files: Boolean = true,
    val download_mode: String = "",
    /** Already redacted by the server. */
    val proxy_url: String = "",
    val poll_minutes: Int = 30,
    val library_dir: String = "",
    val download_dir: String = "",
    val os: String = "",
)

/** `PUT /api/settings` is partial: only non-null fields are changed. */
@Serializable
data class SettingsInput(
    val max_concurrent_downloads: Int? = null,
    val auto_cleanup_empty_anime: Boolean? = null,
    val keep_source_files: Boolean? = null,
    val poll_minutes: Int? = null,
    val library_dir: String? = null,
)

@Serializable
data class SettingsSaveResult(
    val cleaned_anime: Int = 0,
    val cleanup_warning: String = "",
    val library_warning: String = "",
)

const val MIN_POLL_MINUTES = 5
const val MAX_POLL_MINUTES = 1440
const val MAX_CONCURRENT_DOWNLOADS = 32
