package com.autoanime.android.core

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import okhttp3.Request
import okhttp3.Response
import okhttp3.OkHttpClient
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.RequestBody.Companion.toRequestBody
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

@Serializable data class AnimePage(val items: List<Anime>, val next_before: Long)
@Serializable data class Anime(val id: Long, val title: String, val season: Int = 0, val cover: String = "", val meta_name: String = "", val meta_name_cn: String = "", val summary: String = "")
@Serializable data class LoginResult(val ok: Boolean)
@Serializable data class AnimeDetail(val id: Long, val title: String, val cover: String = "", val summary: String = "", val seasons: List<Season> = emptyList())
@Serializable data class Season(val season: Int, val episodes: List<Episode> = emptyList(), val loose: List<MediaItem> = emptyList())
@Serializable data class Episode(val episode: String, val items: List<MediaItem> = emptyList())
@Serializable data class MediaItem(val id: Long, val title: String, val status: String, val episode: String = "", val quality: String = "", val file_path: String = "")
@Serializable data class SetupState(val required: Boolean, val download_mode: String? = null)

open class AnimeApi(internal val client: OkHttpClient, private val connection: ServerConnection) {
    private val json = Json { ignoreUnknownKeys = true }
    open suspend fun login(user: String, password: String): LoginResult {
        val response = request("api/login", "POST", """{"username":${quote(user)},"password":${quote(password)}}""")
        if (!response.successful) throw ApiError(response.code, "Login failed")
        val result = json.decodeFromString<LoginResult>(response.body)
        if (!result.ok) throw ApiError(response.code, "Login failed")
        return result
    }
    open suspend fun setupStatus(): SetupState = get("api/setup/status")
    open suspend fun page(before: Long? = null): AnimePage = get("api/animes?page=1&limit=30" + (before?.let { "&before=$it" } ?: ""))
    open suspend fun detail(id: Long): AnimeDetail = get("api/animes/$id")
    open suspend fun logout() { val response = request("api/logout", "POST"); if (!response.successful) throw ApiError(response.code, "Logout failed") }
    open suspend fun probeSession() { val response = request("api/animes?page=1&limit=1", "GET"); if (response.code == 401) throw ApiError(401, "Session expired"); if (!response.successful) throw ApiError(response.code, "Request failed") }
    private suspend inline fun <reified T> get(path: String): T {
        val response = request(path)
        if (!response.successful) throw ApiError(response.code, "Request failed")
        return json.decodeFromString<T>(response.body)
    }
    private suspend fun request(path: String, method: String = "GET", body: String? = null): ApiResponse = suspendCancellableCoroutine { continuation ->
        val url = connection.resolve(path)
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
private data class ApiResponse(val code: Int, val successful: Boolean, val body: String)

data class ApiError(val status: Int, override val message: String) : Exception(message)

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
