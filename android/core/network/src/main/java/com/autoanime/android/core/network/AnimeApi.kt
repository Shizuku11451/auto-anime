package com.autoanime.android.core.network

import com.autoanime.android.core.model.AggregateResult
import com.autoanime.android.core.model.AnimeDetail
import com.autoanime.android.core.model.AnimeSaveResult
import com.autoanime.android.core.model.AnimeUpdateInput
import com.autoanime.android.core.model.ApplyRulesResult
import com.autoanime.android.core.model.BindInput
import com.autoanime.android.core.model.CorrectInput
import com.autoanime.android.core.model.CorrectResult
import com.autoanime.android.core.model.DiscoverResult
import com.autoanime.android.core.model.Feed
import com.autoanime.android.core.model.FeedInput
import com.autoanime.android.core.model.FeedSaveResult
import com.autoanime.android.core.model.MetaSearchResult
import com.autoanime.android.core.model.PreviewInput
import com.autoanime.android.core.model.PreviewResult
import com.autoanime.android.core.model.RematchResult
import com.autoanime.android.core.model.ServerSettings
import com.autoanime.android.core.model.SettingsInput
import com.autoanime.android.core.model.SettingsSaveResult
import com.autoanime.android.core.model.SyncResult
import com.autoanime.android.core.model.AnimePage
import com.autoanime.android.core.model.DownloadAction
import com.autoanime.android.core.model.DownloadActionResult
import com.autoanime.android.core.model.DownloadPage
import com.autoanime.android.core.model.LoginResult
import com.autoanime.android.core.model.RuntimeStatus
import com.autoanime.android.core.model.SetupState
import kotlinx.serialization.json.Json
import okhttp3.HttpUrl
import okhttp3.Request
import okhttp3.Response
import okhttp3.OkHttpClient
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.RequestBody.Companion.toRequestBody
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

open class AnimeApi(val client: OkHttpClient, private val connection: ServerConnection) {
    /** Null optional fields are omitted: for partial updates the server keeps their stored values. */
    private val json = Json { ignoreUnknownKeys = true; explicitNulls = false }
    open suspend fun login(user: String, password: String): LoginResult {
        val response = request("api/login", "POST", """{"username":${quote(user)},"password":${quote(password)}}""")
        if (!response.successful) throw ApiError(response.code, "Login failed")
        val result = json.decodeFromString<LoginResult>(response.body)
        if (!result.ok) throw ApiError(response.code, "Login failed")
        return result
    }
    open suspend fun setupStatus(): SetupState = get("api/setup/status")
    /**
     * [day] filters by schedule bucket (see [com.autoanime.android.core.model.ScheduleDay]); [today] is the
     * client's calendar date, which decides whether a show has finished airing.
     */
    open suspend fun page(before: Long? = null, day: Int? = null, today: String? = null, counts: Boolean = false): AnimePage = get(
        connection.resolve("api/animes").newBuilder()
            .addQueryParameter("page", "1")
            .addQueryParameter("limit", "30")
            .apply {
                before?.let { addQueryParameter("before", it.toString()) }
                day?.let { addQueryParameter("day", it.toString()) }
                today?.let { addQueryParameter("today", it) }
                if (counts) addQueryParameter("counts", "1")
            }
            .build(),
    )
    open suspend fun detail(id: Long): AnimeDetail = get("api/animes/$id")
    open suspend fun items(group: String?, query: String?, before: Long?): DownloadPage = get(
        connection.resolve("api/items").newBuilder()
            .addQueryParameter("page", "1")
            .apply {
                group?.let { addQueryParameter("group", it) }
                query?.takeIf { it.isNotBlank() }?.let { addQueryParameter("q", it.trim()) }
                before?.let { addQueryParameter("before", it.toString()) }
            }
            .build(),
    )
    open suspend fun runtimeStatus(): RuntimeStatus = get("api/status")
    /** Returns whether the server changed the task; `false` means it was already in that state. */
    open suspend fun itemAction(id: Long, action: DownloadAction): Boolean {
        val response = request(connection.resolve("api/items/$id/${action.path}"), "POST")
        if (!response.successful) throw response.error()
        return json.decodeFromString<DownloadActionResult>(response.body).changed
    }
    /** Removes only the record; the server keeps saved files and downloader sources. */
    open suspend fun deleteItem(id: Long) {
        val response = request(connection.resolve("api/items/$id"), "DELETE")
        if (!response.successful) throw response.error()
    }
    open suspend fun feeds(): List<Feed> = get("api/feeds")
    open suspend fun createFeed(input: FeedInput): FeedSaveResult = send("api/feeds", "POST", json.encodeToString(input))
    open suspend fun updateFeed(id: Long, input: FeedInput): FeedSaveResult = send("api/feeds/$id", "PUT", json.encodeToString(input))
    /** Deletes the subscription and its item records; saved media stays on the server. */
    open suspend fun deleteFeed(id: Long) { send<Unit>("api/feeds/$id", "DELETE", null) }
    open suspend fun syncFeed(id: Long): SyncResult = send("api/sync/$id", "POST", null)
    open suspend fun preview(input: PreviewInput): PreviewResult = send("api/preview", "POST", json.encodeToString(input))

    open suspend fun discoverSubjects(keyword: String): DiscoverResult = get(
        connection.resolve("api/discover/subjects").newBuilder().addQueryParameter("keyword", keyword).build(),
    )
    open suspend fun discoverResources(subjectId: Long): AggregateResult = get(
        connection.resolve("api/discover/resources").newBuilder().addQueryParameter("subject_id", subjectId.toString()).build(),
    )

    open suspend fun updateAnime(id: Long, input: AnimeUpdateInput): AnimeSaveResult = send("api/animes/$id", "PUT", json.encodeToString(input))
    open suspend fun applyRules(id: Long): ApplyRulesResult = send("api/animes/$id/apply-rules", "POST", null)
    open suspend fun rematch(id: Long): RematchResult = send("api/animes/$id/rematch", "POST", null)
    open suspend fun searchMetadata(query: String): MetaSearchResult = get(
        connection.resolve("api/meta/search").newBuilder().addQueryParameter("q", query).build(),
    )
    /** [bangumiId] 0 removes the binding. */
    open suspend fun bindMetadata(id: Long, bangumiId: Long) { send<Unit>("api/animes/$id/metadata", "PUT", json.encodeToString(BindInput(bangumiId))) }
    open suspend fun correctItem(id: Long, input: CorrectInput): CorrectResult = send("api/items/$id", "PATCH", json.encodeToString(input))

    open suspend fun settings(): ServerSettings = get("api/settings")
    open suspend fun saveSettings(input: SettingsInput): SettingsSaveResult = send("api/settings", "PUT", json.encodeToString(input))

    open suspend fun logout() { val response = request("api/logout", "POST"); if (!response.successful) throw ApiError(response.code, "Logout failed") }
    open suspend fun probeSession() { val response = request("api/animes?page=1&limit=1", "GET"); if (response.code == 401) throw ApiError(401, "Session expired"); if (!response.successful) throw ApiError(response.code, "Request failed") }
    private suspend inline fun <reified T> get(path: String): T = get(connection.resolve(path))
    private suspend inline fun <reified T> get(url: HttpUrl): T {
        val response = request(url)
        if (!response.successful) throw ApiError(response.code, "Request failed")
        return json.decodeFromString<T>(response.body)
    }
    /** Writes carry the server's plain-text reason on failure; [O] is [Unit] when the body is ignored. */
    private suspend inline fun <reified O> send(path: String, method: String, body: String?): O {
        val response = request(connection.resolve(path), method, body)
        if (!response.successful) throw response.error()
        return if (O::class == Unit::class) Unit as O else json.decodeFromString<O>(response.body)
    }
    private suspend fun request(path: String, method: String = "GET", body: String? = null): ApiResponse =
        request(connection.resolve(path), method, body)
    private suspend fun request(url: HttpUrl, method: String = "GET", body: String? = null): ApiResponse = suspendCancellableCoroutine { continuation ->
        connection.requireAccepted(url)
        val request = Request.Builder().url(url).method(method, body?.toRequestBody(JSON) ?: if (method == "GET") null else "".toRequestBody(JSON)).build()
        val call = client.newCall(request)
        continuation.invokeOnCancellation { call.cancel() }
        call.enqueue(object : okhttp3.Callback {
            override fun onFailure(call: okhttp3.Call, e: java.io.IOException) { if (continuation.isActive) continuation.resumeWithException(e) }
            override fun onResponse(call: okhttp3.Call, response: Response) {
                try {
                    val result = ApiResponse(response.code, response.isSuccessful, response.body?.boundedString().orEmpty())
                    if (continuation.isActive) continuation.resume(result)
                } catch (e: Exception) {
                    if (continuation.isActive) continuation.resumeWithException(e)
                } finally { response.close() }
            }
        })
    }
    private fun quote(value: String) = kotlinx.serialization.json.JsonPrimitive(value).toString()
    companion object { private val JSON = "application/json; charset=utf-8".toMediaType() }
}
private data class ApiResponse(val code: Int, val successful: Boolean, val body: String) {
    /** Action endpoints answer with a short plain-text reason, such as an invalid state transition. */
    fun error() = ApiError(code, "Request failed", detail = body.trim().takeIf { it.isNotEmpty() && !it.startsWith("<") }?.take(200))
}

/** [detail] is the server's own explanation; like the web client, the app shows it as-is. */
data class ApiError(val status: Int, override val message: String, val detail: String? = null) : Exception(message)

private fun okhttp3.ResponseBody.boundedString(): String {
    val out = java.io.ByteArrayOutputStream()
    byteStream().use { input ->
        val buffer = ByteArray(16 * 1024)
        var total = 0
        while (true) {
            val count = input.read(buffer)
            if (count < 0) break
            total += count
            require(total <= 1_048_576) { "Response is too large" }
            out.write(buffer, 0, count)
        }
    }
    return out.toByteArray().toString(Charsets.UTF_8)
}
