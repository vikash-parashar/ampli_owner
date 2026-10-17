package com.vikash.ampli.owner.ui

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color

// Owner = royal violet (brand final-v2). Products: MP3 rose, Car Hub cyan, Naukri saffron.
val Violet = Color(0xFF5B3FD1)
val VioletLight = Color(0xFF9B85FF)
val Rose = Color(0xFFD6336C)
val Cyan = Color(0xFF0B8FA8)
val Saffron = Color(0xFFC2410C)
val Good = Color(0xFF1F8A4C)
val Warn = Color(0xFFB26A00)
val Bad = Color(0xFFC2352B)
val Star = Color(0xFFF59E0B)

/** Light-mode product colour, for small dots. Use [AmpliColors.product] where dark mode matters. */
fun productColor(id: String?) = when (id) {
    "mp3" -> Rose
    "carhub" -> Cyan
    "naukri" -> Saffron
    else -> Violet
}

/** The preview's design tokens (owner-mock CSS variables), light and dark. */
data class AmpliColors(
    val violet: Color,
    val rose: Color,
    val cyan: Color,
    val saffron: Color,
    val good: Color,
    val warn: Color,
    val bad: Color,
    val goodBg: Color,
    val warnBg: Color,
    val badBg: Color,
    val surface2: Color,
    val line: Color,
    val faint: Color,
    val muted: Color,
    val text: Color,
    val dark: Boolean,
) {
    fun product(id: String?): Color = when (id) {
        "mp3" -> rose
        "carhub" -> cyan
        "naukri" -> saffron
        else -> violet
    }
}

val LightAmpli = AmpliColors(
    violet = Color(0xFF5B3FD1), rose = Color(0xFFD6336C), cyan = Color(0xFF0B8FA8), saffron = Color(0xFFC2410C),
    good = Color(0xFF15803D), warn = Color(0xFFB45309), bad = Color(0xFFB91C1C),
    goodBg = Color(0xFFDCFCE7), warnBg = Color(0xFFFEF3C7), badBg = Color(0xFFFEE2E2),
    surface2 = Color(0xFFF7F5FE), line = Color(0xFFE6E1F6), faint = Color(0xFFA39DBB),
    muted = Color(0xFF6B6585), text = Color(0xFF17132B), dark = false,
)

val DarkAmpli = AmpliColors(
    violet = Color(0xFF8F78FF), rose = Color(0xFFFF5C93), cyan = Color(0xFF3CC6DE), saffron = Color(0xFFFB8A4C),
    good = Color(0xFF4ADE80), warn = Color(0xFFFBBF24), bad = Color(0xFFF87171),
    goodBg = Color(0xFF4ADE80).copy(alpha = 0.14f), warnBg = Color(0xFFFBBF24).copy(alpha = 0.14f),
    badBg = Color(0xFFF87171).copy(alpha = 0.14f),
    surface2 = Color(0xFF1E1936), line = Color(0xFF2B2547), faint = Color(0xFF6E6790),
    muted = Color(0xFFA79FC8), text = Color(0xFFF1EEFF), dark = true,
)

val LocalAmpli = staticCompositionLocalOf<AmpliColors> { LightAmpli }

/** The selected product's colour (animated). Set by OwnerApp. */
val LocalAccent = compositionLocalOf<Color> { Violet }

@Composable
fun ampli(): AmpliColors = LocalAmpli.current

@Composable
fun accent(): Color = LocalAccent.current

private val Light = lightColorScheme(
    primary = Violet, onPrimary = Color.White,
    primaryContainer = Color(0xFFECE7FF), onPrimaryContainer = Color(0xFF1D1060),
    secondary = Color(0xFF6B6585), background = Color(0xFFF4F2FB), surface = Color(0xFFF4F2FB),
    surfaceContainer = Color(0xFFFFFFFF), surfaceContainerLow = Color(0xFFFFFFFF),
    onBackground = Color(0xFF17132B), onSurface = Color(0xFF17132B), onSurfaceVariant = Color(0xFF6B6585),
    outlineVariant = Color(0xFFE6E1F6), error = Bad,
)

private val Dark = darkColorScheme(
    primary = Color(0xFF8F78FF), onPrimary = Color.White,
    primaryContainer = Color(0xFF2C2450), onPrimaryContainer = Color(0xFFE6DFFF),
    secondary = Color(0xFFA79FC8), background = Color(0xFF0E0B1A), surface = Color(0xFF0E0B1A),
    surfaceContainer = Color(0xFF17132A), surfaceContainerLow = Color(0xFF17132A),
    onBackground = Color(0xFFF1EEFF), onSurface = Color(0xFFF1EEFF), onSurfaceVariant = Color(0xFFA79FC8),
    outlineVariant = Color(0xFF2B2547), error = Color(0xFFF87171),
)

@Composable
fun OwnerTheme(content: @Composable () -> Unit) {
    val dark = isSystemInDarkTheme()
    CompositionLocalProvider(LocalAmpli provides (if (dark) DarkAmpli else LightAmpli)) {
        MaterialTheme(colorScheme = if (dark) Dark else Light, content = content)
    }
}
