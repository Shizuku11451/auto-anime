package com.autoanime.android.core.data

import com.autoanime.android.core.model.AppError
import com.autoanime.android.core.network.AnimeApi
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.withContext

/** The session currently connected, or null. */
val SessionRepository.connected: Session? get() = (state.value as? SessionState.Connected)?.session

sealed interface CallResult<out T> {
    data class Success<T>(val value: T) : CallResult<T>
    data class Failure(val error: AppError) : CallResult<Nothing>
    /** The session changed or expired while the call ran; the result must not be shown. */
    data object Stale : CallResult<Nothing>
}

/**
 * Runs [block] against [owner]'s API on [io]. A 401 ends that session once; a result that arrives
 * after [owner] stopped being the connected session is reported as [CallResult.Stale].
 */
suspend fun <T> SessionRepository.call(
    owner: Session,
    io: CoroutineDispatcher,
    block: suspend AnimeApi.() -> T,
): CallResult<T> {
    val result = try {
        CallResult.Success(withContext(io) { owner.api.block() })
    } catch (e: CancellationException) {
        throw e
    } catch (e: Exception) {
        val error = e.toAppError()
        if (error == AppError.SessionExpired) {
            onUnauthorized(owner)
            return CallResult.Stale
        }
        CallResult.Failure(error)
    }
    return if (connected === owner) result else CallResult.Stale
}
