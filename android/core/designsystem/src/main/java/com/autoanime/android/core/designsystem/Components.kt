package com.autoanime.android.core.designsystem

import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.painter.ColorPainter
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import coil3.ImageLoader
import coil3.compose.AsyncImage
import com.autoanime.android.core.model.AppError

/** The current session's cover loader; it carries that session's cookie and in-memory cache. */
val LocalCoverImageLoader = compositionLocalOf<ImageLoader?> { null }

@Composable
fun CoverImage(url: String?, contentDescription: String?, modifier: Modifier = Modifier) {
    val loader = LocalCoverImageLoader.current
    val placeholder = ColorPainter(MaterialTheme.colorScheme.surfaceContainerHighest)
    if (url == null || loader == null) {
        Image(placeholder, contentDescription, modifier)
        return
    }
    AsyncImage(
        model = url,
        imageLoader = loader,
        contentDescription = contentDescription,
        contentScale = ContentScale.Crop,
        placeholder = placeholder,
        error = placeholder,
        modifier = modifier,
    )
}

@Composable
fun LoadingState(modifier: Modifier = Modifier) {
    Box(modifier.fillMaxSize(), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
}

@Composable
fun ErrorState(error: AppError, onRetry: (() -> Unit)?, modifier: Modifier = Modifier) {
    Column(
        modifier.fillMaxSize().padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp, Alignment.CenterVertically),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(error.message(), style = MaterialTheme.typography.bodyLarge, textAlign = TextAlign.Center)
        if (onRetry != null) FilledTonalButton(onClick = onRetry) { Text(stringResource(R.string.retry)) }
    }
}

@Composable
fun AppError.message(): String = when (this) {
    AppError.SessionExpired -> stringResource(R.string.error_session_expired)
    AppError.SetupRequired -> stringResource(R.string.error_setup_required)
    AppError.InvalidSavedUrl -> stringResource(R.string.error_invalid_saved_url)
    AppError.UnsupportedServer -> stringResource(R.string.error_unsupported_server)
    AppError.ResponseTooLarge -> stringResource(R.string.error_response_too_large)
    AppError.Network -> stringResource(R.string.error_network)
    is AppError.Http -> stringResource(R.string.error_http, status)
}
