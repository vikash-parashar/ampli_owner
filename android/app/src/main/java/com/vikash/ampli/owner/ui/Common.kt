package com.vikash.ampli.owner.ui

import com.vikash.ampli.owner.R
import androidx.compose.ui.res.painterResource
import androidx.compose.foundation.Image
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import java.util.Locale

/**
 * Loads [load] whenever [key] changes and shows a spinner, an error with retry, or [content].
 * With [keepStale] the last result stays on screen while a reload runs (no spinner flash after an action).
 */
@Composable
fun <T> Loadable(
    key: Any,
    load: suspend () -> T,
    onRetry: () -> Unit,
    keepStale: Boolean = false,
    content: @Composable (T) -> Unit,
) {
    val state by produceState<Result<T>?>(initialValue = null, key) {
        if (!keepStale) value = null
        value = runCatching { load() }
    }
    when (val r = state) {
        null -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
        else -> r.fold(
            onSuccess = { content(it) },
            onFailure = { e ->
                Column(
                    Modifier.fillMaxSize().padding(24.dp),
                    verticalArrangement = Arrangement.Center,
                    horizontalAlignment = Alignment.CenterHorizontally,
                ) {
                    Text("Couldn't load this", style = MaterialTheme.typography.titleMedium)
                    Text(e.message ?: e.javaClass.simpleName, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Button(onClick = onRetry, modifier = Modifier.padding(top = 12.dp)) { Text("Try again") }
                }
            },
        )
    }
}

@Composable
fun Section(title: String) {
    Text(
        title.uppercase(),
        style = MaterialTheme.typography.labelMedium.copy(letterSpacing = 1.sp, fontWeight = FontWeight.SemiBold),
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(start = 4.dp, top = 8.dp),
    )
}

@Composable
fun OwnerCard(modifier: Modifier = Modifier, onClick: (() -> Unit)? = null, content: @Composable ColumnScope.() -> Unit) {
    val colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainer)
    val shape = RoundedCornerShape(18.dp)
    if (onClick != null) {
        Card(onClick = onClick, modifier = modifier.fillMaxWidth(), shape = shape, colors = colors) {
            Column(Modifier.padding(14.dp), content = content)
        }
    } else {
        Card(modifier = modifier.fillMaxWidth(), shape = shape, colors = colors) {
            Column(Modifier.padding(14.dp), content = content)
        }
    }
}

@Composable
fun Dot(color: Color, size: Int = 10) {
    Box(Modifier.size(size.dp).background(color, CircleShape))
}

@Composable
fun Pill(text: String, color: Color) {
    Surface(color = color.copy(alpha = 0.14f), shape = RoundedCornerShape(50)) {
        Text(
            text, color = color, style = MaterialTheme.typography.labelMedium,
            modifier = Modifier.padding(horizontal = 9.dp, vertical = 3.dp),
        )
    }
}

val kindLabel = mapOf("bug" to "Bug", "feature" to "Feature", "support" to "Support", "review" to "Review", "feedback" to "Feedback")
fun kindColor(kind: String) = when (kind) {
    "bug" -> Bad
    "feature" -> Violet
    "support" -> Warn
    else -> Good
}

val statusLabel = linkedMapOf(
    "new" to "New", "waiting_on_owner" to "Waiting on me", "waiting_on_user" to "Waiting on user",
    "planned" to "Planned", "in_progress" to "In progress", "fixed" to "Fixed", "closed" to "Closed",
)

// ---------------- the dashboard kit (matches the owner-mock preview) ----------------

fun f1(d: Double): String = String.format(Locale.US, "%.1f", d)
fun f2(d: Double): String = String.format(Locale.US, "%.2f", d)

/** "▲ +22%" or "▼ -5%". */
fun changeText(pct: Double): String {
    val rounded = Math.round(pct)
    return if (pct >= 0) "▲ +$rounded%" else "▼ $rounded%"
}

/** A scrolling column for the animated screens. Scrolls back to the top when [resetKey] changes. */
@Composable
fun ScrollPage(modifier: Modifier, resetKey: Any?, content: @Composable ColumnScope.() -> Unit) {
    val scroll = rememberScrollState()
    LaunchedEffect(resetKey) { scroll.animateScrollTo(0) }
    Column(
        modifier
            .fillMaxSize()
            .verticalScroll(scroll)
            .padding(start = 16.dp, end = 16.dp, top = 6.dp, bottom = 104.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
        content = content,
    )
}

/** The preview's white rounded card with a soft shadow. */
@Composable
fun AmpliCard(modifier: Modifier = Modifier, content: @Composable ColumnScope.() -> Unit) {
    Surface(
        modifier = modifier.fillMaxWidth(),
        shape = RoundedCornerShape(20.dp),
        color = MaterialTheme.colorScheme.surfaceContainer,
        shadowElevation = 2.dp,
    ) {
        Column(Modifier.padding(16.dp), content = content)
    }
}

/** Card heading: title on the left, a small grey note on the right. */
@Composable
fun CardTitle(title: String, aside: String? = null, modifier: Modifier = Modifier) {
    Row(modifier.fillMaxWidth().padding(bottom = 12.dp), verticalAlignment = Alignment.CenterVertically) {
        Text(title, fontSize = 14.sp, fontWeight = FontWeight.ExtraBold, modifier = Modifier.weight(1f))
        if (aside != null) {
            Text(aside, fontSize = 11.5.sp, fontWeight = FontWeight.SemiBold, color = ampli().muted, textAlign = TextAlign.End)
        }
    }
}

/** A rounded label chip. */
@Composable
fun Chip(text: String, background: Color, color: Color, modifier: Modifier = Modifier) {
    Text(
        text,
        color = color,
        fontSize = 11.sp,
        fontWeight = FontWeight.ExtraBold,
        maxLines = 1,
        modifier = modifier
            .clip(RoundedCornerShape(50))
            .background(background)
            .padding(horizontal = 8.dp, vertical = 3.dp),
    )
}

enum class BtnKind { Primary, Ghost, Ok, Bad }

/** The preview's small rounded button (.btn, .btn.ghost, .btn.ok, .btn.badb). */
@Composable
fun SmallButton(text: String, kind: BtnKind = BtnKind.Primary, modifier: Modifier = Modifier, enabled: Boolean = true, onClick: () -> Unit) {
    val a = ampli()
    val primary = accent()
    val colors: Pair<Color, Color> = when (kind) {
        BtnKind.Primary -> Pair(primary, Color.White)
        BtnKind.Ghost -> Pair(a.surface2, a.text)
        BtnKind.Ok -> Pair(a.goodBg, a.good)
        BtnKind.Bad -> Pair(a.badBg, a.bad)
    }
    val bg = colors.first
    val fg = colors.second
    Text(
        text,
        color = fg,
        fontSize = 12.5.sp,
        fontWeight = FontWeight.ExtraBold,
        maxLines = 1,
        textAlign = TextAlign.Center,
        modifier = modifier
            .alpha(if (enabled) 1f else 0.5f)
            .clip(RoundedCornerShape(11.dp))
            .background(bg)
            .clickable(enabled = enabled, onClick = onClick)
            .padding(horizontal = 14.dp, vertical = 8.dp),
    )
}

/**
 * A stat box from the 2x2 grid. Label sits top-left; the number (and its note) sits bottom-right,
 * right-aligned. An optional sparkline sits bottom-left.
 */
@Composable
fun StatBox(
    label: String,
    modifier: Modifier = Modifier,
    delta: String? = null,
    deltaColor: Color = Color.Unspecified,
    spark: List<Float>? = null,
    sparkKey: Any? = null,
    flat: Boolean = false,
    value: @Composable () -> Unit,
) {
    val a = ampli()
    val dotColor = accent()
    Surface(
        modifier = modifier,
        shape = RoundedCornerShape(18.dp),
        color = if (flat) a.surface2 else MaterialTheme.colorScheme.surfaceContainer,
        shadowElevation = if (flat) 0.dp else 2.dp,
    ) {
        Column(
            Modifier
                .fillMaxWidth()
                .heightIn(min = 96.dp)
                .padding(horizontal = 14.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.SpaceBetween,
        ) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Box(Modifier.size(8.dp).background(dotColor, CircleShape))
                Spacer(Modifier.width(6.dp))
                Text(
                    label, fontSize = 11.5.sp, fontWeight = FontWeight.Bold, color = a.muted,
                    maxLines = 1, overflow = TextOverflow.Ellipsis, textAlign = TextAlign.Start,
                )
            }
            Spacer(Modifier.height(8.dp))
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.Bottom) {
                if (spark != null && spark.size >= 2) {
                    Sparkline(spark, dotColor, sparkKey, Modifier.padding(bottom = 2.dp).size(width = 58.dp, height = 26.dp))
                }
                Spacer(Modifier.weight(1f))
                Column(horizontalAlignment = Alignment.End) {
                    value()
                    if (delta != null) {
                        Text(delta, fontSize = 11.sp, fontWeight = FontWeight.Bold, color = deltaColor, textAlign = TextAlign.End, maxLines = 1)
                    }
                }
            }
        }
    }
}

/** Big right-aligned number used inside [StatBox]. */
@Composable
fun StatNumber(text: String, color: Color = Color.Unspecified) {
    Text(text, fontSize = 24.sp, fontWeight = FontWeight.ExtraBold, letterSpacing = (-0.6).sp, textAlign = TextAlign.End, maxLines = 1, color = color)
}

/** A small legend row: coloured square + label. */
@Composable
fun Legend(items: List<Pair<String, Color>>, modifier: Modifier = Modifier) {
    Row(
        modifier.fillMaxWidth().padding(top = 8.dp),
        horizontalArrangement = Arrangement.Center,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        items.forEachIndexed { i, (label, color) ->
            if (i > 0) Spacer(Modifier.width(14.dp))
            Box(Modifier.size(10.dp).clip(RoundedCornerShape(3.dp)).background(color))
            Spacer(Modifier.width(5.dp))
            Text(label, fontSize = 11.sp, fontWeight = FontWeight.SemiBold, color = ampli().muted, maxLines = 1)
        }
    }
}

/** The product's app icon (MP3, Car Hub), or a rounded square with its colour and initial (Naukri). */
@Composable
fun ProductBadge(id: String, size: Dp = 36.dp, onAccent: Boolean = false) {
    val icon = when (id) {
        "mp3" -> R.drawable.logo_mp3
        "carhub" -> R.drawable.logo_carhub
        else -> 0
    }
    if (icon != 0) {
        Image(
            painterResource(icon),
            contentDescription = null,
            modifier = Modifier.size(size).clip(RoundedCornerShape(size * 0.3f)),
        )
        return
    }
    val letter = when (id) {
        "mp3" -> "M"
        "carhub" -> "C"
        "naukri" -> "N"
        else -> "A"
    }
    val bg = if (onAccent) Color.White.copy(alpha = 0.25f) else ampli().product(id)
    Box(
        Modifier.size(size).clip(RoundedCornerShape(size * 0.3f)).background(bg),
        contentAlignment = Alignment.Center,
    ) {
        Text(letter, color = Color.White, fontWeight = FontWeight.ExtraBold, fontSize = (size.value * 0.42f).sp, maxLines = 1)
    }
}

/** The warning chip in the top bar while the app shows sample numbers. */
@Composable
fun SampleChip() {
    val a = ampli()
    Text(
        "SAMPLE DATA",
        fontSize = 10.sp,
        fontWeight = FontWeight.ExtraBold,
        letterSpacing = 0.8.sp,
        color = a.warn,
        maxLines = 1,
        modifier = Modifier
            .padding(end = 4.dp)
            .clip(RoundedCornerShape(50))
            .background(a.warnBg)
            .padding(horizontal = 8.dp, vertical = 4.dp),
    )
}

private val switcherIds = listOf("all", "mp3", "carhub", "naukri")
private val switcherLabels = listOf("All", "MP3", "Car Hub", "Naukri")

/** All / MP3 / Car Hub / Naukri with a sliding pill in the selected product's colour. */
@Composable
fun ProductSwitcher(selected: String, onSelect: (String) -> Unit, modifier: Modifier = Modifier) {
    val a = ampli()
    val index = switcherIds.indexOf(selected).coerceAtLeast(0)
    val position by animateFloatAsState(
        targetValue = index.toFloat(),
        animationSpec = spring(dampingRatio = 0.72f, stiffness = Spring.StiffnessMediumLow),
        label = "pill",
    )
    val pillColor by animateColorAsState(a.product(selected), tween(350), label = "pillColor")
    Surface(
        modifier = modifier.fillMaxWidth(),
        shape = RoundedCornerShape(14.dp),
        color = MaterialTheme.colorScheme.surfaceContainer,
        shadowElevation = 2.dp,
    ) {
        BoxWithConstraints(Modifier.padding(4.dp)) {
            val cell: Dp = maxWidth / switcherIds.size
            Box(
                Modifier
                    .offset(x = cell * position)
                    .width(cell)
                    .height(36.dp)
                    .clip(RoundedCornerShape(10.dp))
                    .background(pillColor),
            )
            Row(Modifier.fillMaxWidth().height(36.dp)) {
                switcherIds.forEachIndexed { i, id ->
                    val on = i == index
                    val textColor by animateColorAsState(if (on) Color.White else a.muted, tween(300), label = "switchText")
                    Row(
                        Modifier
                            .weight(1f)
                            .fillMaxHeight()
                            .clip(RoundedCornerShape(10.dp))
                            .clickable { onSelect(id) },
                        horizontalArrangement = Arrangement.Center,
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        if (id != "all") {
                            ProductBadge(id, 16.dp, onAccent = on)
                            Spacer(Modifier.width(5.dp))
                        }
                        Text(switcherLabels[i], color = textColor, fontSize = 12.5.sp, fontWeight = FontWeight.Bold, maxLines = 1)
                    }
                }
            }
        }
    }
}

/** The hero card: product-coloured gradient with a floating glow and a shine sweep. */
@Composable
fun HeroCard(modifier: Modifier = Modifier, content: @Composable ColumnScope.() -> Unit) {
    val color = accent()
    Surface(
        modifier = modifier.fillMaxWidth(),
        shape = RoundedCornerShape(20.dp),
        color = Color.Transparent,
        shadowElevation = 3.dp,
    ) {
        Box(Modifier.heroBackground(color)) {
            Column(Modifier.fillMaxWidth().padding(18.dp), content = content)
        }
    }
}
