package com.autoanime.android.core.model

import kotlinx.serialization.Serializable

/**
 * One release from `/api/items`. The release link is deliberately not decoded: it can carry
 * a magnet or a private tracker passkey, and the client never needs it.
 */
@Serializable
data class DownloadItem(
    val id: Long,
    val title: String = "",
    val status: String = "",
    val anime_title: String = "",
    val episode: String = "",
    val quality: String = "",
    val fansub_group: String = "",
    val feed_name: String = "",
    val downloader: String = "",
    val task_error: String = "",
    val progress: Int = 0,
    val download_speed: Long = 0,
    val file_path: String = "",
    val cover: String = "",
)

@Serializable data class DownloadPage(val items: List<DownloadItem>, val next_before: Long)

/** The part of `/api/status` the client reads: item counts by raw status, across the library. */
@Serializable data class RuntimeStatus(val counts: Map<String, Int> = emptyMap())

@Serializable data class DownloadActionResult(val changed: Boolean)

enum class DownloadAction(val path: String) { Download("download"), Pause("pause"), Resume("resume") }

/** An empty status means the server has not picked the item up yet. */
val DownloadItem.effectiveStatus: String get() = status.ifEmpty { STATUS_PENDING }

/** Mirrors `web/src/utils/status.ts`; the server still rejects any transition it does not allow. */
fun isDownloadingStatus(status: String): Boolean =
    status == STATUS_DOWNLOADING || status == STATUS_RESUMING || status.startsWith(SENT_TO_PREFIX)

fun isQueuedStatus(status: String): Boolean = status == STATUS_QUEUED || status == STATUS_QUEUED_RESUME

fun DownloadItem.canRetry(): Boolean = effectiveStatus in setOf(STATUS_PENDING, STATUS_FAILED, STATUS_WAITING_DOWNLOADER)

fun DownloadItem.canPause(): Boolean = effectiveStatus.let {
    isDownloadingStatus(it) || isQueuedStatus(it) || it == STATUS_PENDING || it == STATUS_WAITING_DOWNLOADER
}

fun DownloadItem.canResume(): Boolean = effectiveStatus == STATUS_PAUSED

/** Direct HTTP downloads restart from the beginning after a pause; BT/external tasks keep progress. */
val DownloadItem.restartsOnResume: Boolean get() = downloader == "direct"

fun DownloadItem.isPlayable(): Boolean = effectiveStatus == STATUS_SAVED && file_path.isNotEmpty()

/** Status filters; [query] is the server's `group` value and [matches] classifies a raw status. */
enum class StatusGroup(val query: String?, val matches: (String) -> Boolean) {
    All(null, { true }),
    Pending("待下载", { it == STATUS_PENDING || isQueuedStatus(it) }),
    Downloading("下载中", ::isDownloadingStatus),
    Paused("已暂停", { it == STATUS_PAUSED }),
    Saved("已保存", { it == STATUS_SAVED }),
    Failed("下载失败", { it == STATUS_FAILED }),
    WaitingDownloader("等待下载器", { it == STATUS_WAITING_DOWNLOADER }),
    ;

    fun count(counts: Map<String, Int>): Int = counts.entries.sumOf { (status, n) -> if (matches(status)) n else 0 }
}

/** Whether the server reports work that can still change state without user action. */
fun hasActiveDownloads(counts: Map<String, Int>): Boolean =
    counts.any { (status, n) -> n > 0 && (isDownloadingStatus(status) || isQueuedStatus(status)) }

const val STATUS_PENDING = "待下载"
const val STATUS_QUEUED = "等待下载"
const val STATUS_QUEUED_RESUME = "等待继续"
const val STATUS_PAUSED = "已暂停"
const val STATUS_FAILED = "下载失败"
const val STATUS_WAITING_DOWNLOADER = "等待下载器"
const val STATUS_DOWNLOADING = "下载中"
const val STATUS_RESUMING = "继续中"
/** External downloader tasks read "已发送到 Aria2", "已发送到 qBittorrent", and so on. */
const val SENT_TO_PREFIX = "已发送到"
