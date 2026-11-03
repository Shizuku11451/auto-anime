package com.autoanime.android.core.designsystem

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.ColorScheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext

// Mirrors web/src/styles/tokens.css: low-saturation teal accent on green-tinted neutrals.
private val LightColors = lightColorScheme(
    primary = Color(0xFF167451),
    onPrimary = Color(0xFFFFFFFF),
    primaryContainer = Color(0xFFD3EBDF),
    onPrimaryContainer = Color(0xFF105C40),
    secondary = Color(0xFF52665A),
    onSecondary = Color(0xFFFFFFFF),
    secondaryContainer = Color(0xFFE5ECE7),
    onSecondaryContainer = Color(0xFF1E2C24),
    background = Color(0xFFF6F8F6),
    onBackground = Color(0xFF1E2C24),
    surface = Color(0xFFF6F8F6),
    onSurface = Color(0xFF1E2C24),
    surfaceVariant = Color(0xFFE5ECE7),
    onSurfaceVariant = Color(0xFF52665A),
    surfaceContainerLowest = Color(0xFFFFFFFF),
    surfaceContainerLow = Color(0xFFFFFFFF),
    surfaceContainer = Color(0xFFF1F5F2),
    surfaceContainerHigh = Color(0xFFEEF3EF),
    surfaceContainerHighest = Color(0xFFE5ECE7),
    outline = Color(0xFF65796B),
    outlineVariant = Color(0xFFD5DDD8),
    error = Color(0xFFDC2626),
    onError = Color(0xFFFFFFFF),
    errorContainer = Color(0xFFFDE8E8),
    onErrorContainer = Color(0xFF7F1D1D),
)

private val DarkColors = darkColorScheme(
    primary = Color(0xFF62CBA5),
    onPrimary = Color(0xFF102A20),
    primaryContainer = Color(0xFF1E4A39),
    onPrimaryContainer = Color(0xFF86DBB9),
    secondary = Color(0xFFA8B8AE),
    onSecondary = Color(0xFF19211E),
    secondaryContainer = Color(0xFF2B3730),
    onSecondaryContainer = Color(0xFFE8EEEA),
    background = Color(0xFF111615),
    onBackground = Color(0xFFE8EEEA),
    surface = Color(0xFF111615),
    onSurface = Color(0xFFE8EEEA),
    surfaceVariant = Color(0xFF2B3730),
    onSurfaceVariant = Color(0xFFA8B8AE),
    surfaceContainerLowest = Color(0xFF0D1110),
    surfaceContainerLow = Color(0xFF151B19),
    surfaceContainer = Color(0xFF19211E),
    surfaceContainerHigh = Color(0xFF202A25),
    surfaceContainerHighest = Color(0xFF2B3730),
    outline = Color(0xFF8E9E94),
    outlineVariant = Color(0xFF34413A),
    error = Color(0xFFF87171),
    onError = Color(0xFF3B0A0A),
    errorContainer = Color(0xFF5C1D1D),
    onErrorContainer = Color(0xFFFECACA),
)

@Composable
fun AutoAnimeTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    dynamicColor: Boolean = false,
    content: @Composable () -> Unit,
) {
    val colors: ColorScheme = when {
        dynamicColor && supportsDynamicColor -> {
            val context = LocalContext.current
            if (darkTheme) dynamicDarkColorScheme(context) else dynamicLightColorScheme(context)
        }
        darkTheme -> DarkColors
        else -> LightColors
    }
    MaterialTheme(colorScheme = colors, content = content)
}

/** Dynamic color needs Android 12; older systems keep the brand palette. */
val supportsDynamicColor: Boolean get() = Build.VERSION.SDK_INT >= Build.VERSION_CODES.S
