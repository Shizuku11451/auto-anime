package com.autoanime.android.core.model

/** User-facing failure categories; the UI maps each one to a localized message. */
sealed interface AppError {
    data object SessionExpired : AppError
    data object SetupRequired : AppError
    data object InvalidSavedUrl : AppError
    data object UnsupportedServer : AppError
    data object ResponseTooLarge : AppError
    data object Network : AppError
    data class Http(val status: Int) : AppError
}
