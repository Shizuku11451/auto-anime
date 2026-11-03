package com.autoanime.android.core.data

import com.autoanime.android.core.model.AppError
import com.autoanime.android.core.network.ApiError
import kotlinx.serialization.SerializationException

fun Throwable.toAppError(): AppError = when (this) {
    is ApiError -> if (status == 401) AppError.SessionExpired else AppError.Http(status)
    // The server is reachable but predates the bounded mobile endpoints.
    is SerializationException -> AppError.UnsupportedServer
    is IllegalArgumentException -> if (message == "Response is too large") AppError.ResponseTooLarge else AppError.Network
    else -> AppError.Network
}
