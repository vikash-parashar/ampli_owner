package com.vikash.ampli.owner.ui

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

// Owner = royal violet (brand final-v2). Product dots: MP3 rose, Car Hub cyan.
val Violet = Color(0xFF5B3FD1)
val VioletLight = Color(0xFF9B85FF)
val Rose = Color(0xFFD6336C)
val Cyan = Color(0xFF0B8FA8)
val Good = Color(0xFF1F8A4C)
val Warn = Color(0xFFB26A00)
val Bad = Color(0xFFC2352B)

fun productColor(id: String?) = when (id) {
    "mp3" -> Rose
    "carhub" -> Cyan
    else -> Violet
}

private val Light = lightColorScheme(
    primary = Violet, onPrimary = Color.White,
    primaryContainer = Color(0xFFECE7FF), onPrimaryContainer = Color(0xFF1D1060),
    secondary = Color(0xFF6B6584), background = Color(0xFFF8F7FC), surface = Color(0xFFF8F7FC),
    surfaceContainer = Color(0xFFFFFFFF), surfaceContainerLow = Color(0xFFFFFFFF),
    onBackground = Color(0xFF1D1830), onSurface = Color(0xFF1D1830), onSurfaceVariant = Color(0xFF6B6584),
    outlineVariant = Color(0xFFE3DFEE), error = Bad,
)

private val Dark = darkColorScheme(
    primary = VioletLight, onPrimary = Color(0xFF140F26),
    primaryContainer = Color(0xFF2C2450), onPrimaryContainer = Color(0xFFE6DFFF),
    secondary = Color(0xFFA39CBD), background = Color(0xFF15121F), surface = Color(0xFF15121F),
    surfaceContainer = Color(0xFF1F1A2E), surfaceContainerLow = Color(0xFF1F1A2E),
    onBackground = Color(0xFFEFECF8), onSurface = Color(0xFFEFECF8), onSurfaceVariant = Color(0xFFA39CBD),
    outlineVariant = Color(0xFF2F2944), error = Color(0xFFF2766B),
)

@Composable
fun OwnerTheme(content: @Composable () -> Unit) {
    MaterialTheme(colorScheme = if (isSystemInDarkTheme()) Dark else Light, content = content)
}
