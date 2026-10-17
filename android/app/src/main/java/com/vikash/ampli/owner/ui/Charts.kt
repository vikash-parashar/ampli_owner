package com.vikash.ampli.owner.ui

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.AnimationVector1D
import androidx.compose.animation.core.CubicBezierEasing
import androidx.compose.animation.core.Easing
import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.PathMeasure
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.lerp
import androidx.compose.ui.layout.Layout
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.vikash.ampli.owner.data.WeekDownloads
import com.vikash.ampli.owner.data.grouped
import kotlinx.coroutines.delay

// ---------------- easing curves from the preview CSS ----------------

/** easeOutQuart, used by the count-up numbers. */
val EaseOutQuart: Easing = Easing { f ->
    val r = 1f - f
    1f - r * r * r * r
}

/** cubic-bezier(.2,.8,.2,1): card rise, bar widths, donut sweeps. */
val RiseEasing: Easing = CubicBezierEasing(0.2f, 0.8f, 0.2f, 1f)

/** cubic-bezier(.6,0,.2,1): line drawing. */
val DrawEasing: Easing = CubicBezierEasing(0.6f, 0f, 0.2f, 1f)

/** A small overshoot for bars growing in (the preview's cubic-bezier(.2,.9,.3,1.2)). */
val EaseOutBack: Easing = Easing { f ->
    val c1 = 1.2f
    val c3 = c1 + 1f
    val x = f - 1f
    1f + c3 * x * x * x + c1 * x * x
}

/**
 * A 0..1 animation that starts (after [delayMs]) when this is first composed and again whenever [key] changes,
 * e.g. when the owner switches product. Read `.value` inside draw or graphicsLayer blocks where possible.
 */
@Composable
fun rememberEnter(key: Any?, durationMs: Int, delayMs: Int = 0, easing: Easing = LinearEasing): Animatable<Float, AnimationVector1D> {
    val anim = remember(key) { Animatable(0f) }
    LaunchedEffect(anim) {
        if (delayMs > 0) delay(delayMs.toLong())
        anim.animateTo(1f, tween(durationMillis = durationMs, easing = easing))
    }
    return anim
}

/** A time cursor in milliseconds (0..[totalMs]) for charts whose parts start one after another. */
@Composable
fun rememberTimeline(key: Any?, totalMs: Int): Animatable<Float, AnimationVector1D> {
    val anim = remember(key) { Animatable(0f) }
    LaunchedEffect(anim) {
        anim.animateTo(totalMs.toFloat(), tween(durationMillis = totalMs, easing = LinearEasing))
    }
    return anim
}

private fun stagePart(timeMs: Float, startMs: Int, durationMs: Int, easing: Easing): Float {
    val local = ((timeMs - startMs) / durationMs.toFloat()).coerceIn(0f, 1f)
    return easing.transform(local)
}

/** Cards rise in one after another: fade + 14dp slide + a hint of scale, 70 ms apart (the preview's .rise). */
@Composable
fun Modifier.rise(key: Any?, index: Int): Modifier {
    val a = rememberEnter(key, 600, index * 70, RiseEasing)
    return this.graphicsLayer {
        val p = a.value
        alpha = p
        translationY = (1f - p) * 14.dp.toPx()
        val s = 0.985f + 0.015f * p
        scaleX = s
        scaleY = s
    }
}

/** A number that counts up from 0 over 1.2 s with easeOutQuart. */
@Composable
fun CountUpText(
    target: Double,
    key: Any?,
    format: (Double) -> String,
    modifier: Modifier = Modifier,
    style: TextStyle = TextStyle.Default,
    color: Color = Color.Unspecified,
    textAlign: TextAlign = TextAlign.Start,
) {
    val a = rememberEnter(key, 1200, 0, EaseOutQuart)
    Text(format(target * a.value), modifier = modifier, style = style, color = color, textAlign = textAlign, maxLines = 1)
}

/** Draws the first [progress] (0..1) of [path] as a stroke. */
fun DrawScope.drawPartialPath(path: Path, progress: Float, color: Color, width: Float) {
    if (progress <= 0f) return
    val measure = PathMeasure()
    measure.setPath(path, false)
    val segment = Path()
    measure.getSegment(0f, measure.length * progress.coerceAtMost(1f), segment, true)
    drawPath(segment, color, style = Stroke(width = width, cap = StrokeCap.Round, join = StrokeJoin.Round))
}

/** A tiny line that draws itself in. */
@Composable
fun Sparkline(values: List<Float>, color: Color, key: Any?, modifier: Modifier = Modifier) {
    val a = rememberEnter(key, 1400, 200, DrawEasing)
    Canvas(modifier) {
        if (values.size < 2) return@Canvas
        val mx = values.maxOrNull() ?: 0f
        val mn = values.minOrNull() ?: 0f
        val range = if (mx - mn == 0f) 1f else mx - mn
        val sw = 2.dp.toPx()
        val w = size.width
        val h = size.height
        val path = Path()
        values.forEachIndexed { i, v ->
            val x = i * w / (values.size - 1)
            val y = h - sw - (v - mn) / range * (h - 2f * sw)
            if (i == 0) path.moveTo(x, y) else path.lineTo(x, y)
        }
        drawPartialPath(path, a.value, color.copy(alpha = 0.9f), sw)
    }
}

/** 30-day revenue: smooth line drawn in, gradient area fading in, pulsing dot on today. */
@Composable
fun RevenueLineChart(values: List<Long>, color: Color, key: Any?, modifier: Modifier = Modifier) {
    val draw = rememberEnter(key, 1400, 200, DrawEasing)
    val fade = rememberEnter(key, 900, 900, LinearEasing)
    val pulse = rememberInfiniteTransition(label = "pulse").animateFloat(
        initialValue = 5f,
        targetValue = 7f,
        animationSpec = infiniteRepeatable(tween(900, easing = FastOutSlowInEasing), RepeatMode.Reverse),
        label = "pulseRadius",
    )
    val a = ampli()
    val ring = androidx.compose.material3.MaterialTheme.colorScheme.surfaceContainer
    Column(modifier.fillMaxWidth()) {
        Canvas(Modifier.fillMaxWidth().height(132.dp)) {
            val w = size.width
            val pad = 6.dp.toPx()
            val top = 12.dp.toPx()
            val bottom = size.height - 4.dp.toPx()
            val dash = PathEffect.dashPathEffect(floatArrayOf(3.dp.toPx(), 4.dp.toPx()), 0f)
            for (i in 0..3) {
                val y = top + i * (bottom - top) / 3f
                drawLine(a.line, Offset(0f, y), Offset(w, y), strokeWidth = 1.dp.toPx(), pathEffect = dash)
            }
            if (values.size < 2) return@Canvas
            val mx = (values.maxOrNull() ?: 0L).toFloat().coerceAtLeast(1f) * 1.1f
            val pts: List<Offset> = values.mapIndexed { i, v ->
                Offset(pad + i * (w - 2f * pad) / (values.size - 1), bottom - v.toFloat() / mx * (bottom - top))
            }
            val line = Path()
            line.moveTo(pts[0].x, pts[0].y)
            for (i in 1 until pts.size) {
                val p0 = pts[i - 1]
                val p1 = pts[i]
                val cx = (p0.x + p1.x) / 2f
                line.cubicTo(cx, p0.y, cx, p1.y, p1.x, p1.y)
            }
            val area = Path()
            area.addPath(line)
            area.lineTo(pts[pts.size - 1].x, bottom)
            area.lineTo(pts[0].x, bottom)
            area.close()
            drawPath(
                area,
                Brush.verticalGradient(listOf(color.copy(alpha = 0.35f), color.copy(alpha = 0f)), startY = top, endY = bottom),
                alpha = fade.value,
            )
            drawPartialPath(line, draw.value, color, 3.dp.toPx())
            val f = fade.value
            if (f > 0f) {
                val last = pts[pts.size - 1]
                val r = pulse.value.dp.toPx()
                drawCircle(ring, radius = r + 2.5.dp.toPx(), center = last, alpha = f)
                drawCircle(color, radius = r, center = last, alpha = f)
            }
        }
        Row(Modifier.fillMaxWidth().padding(horizontal = 6.dp)) {
            Text("30 days ago", fontSize = 10.sp, color = a.faint)
            Spacer(Modifier.weight(1f))
            Text("Today", fontSize = 10.sp, color = a.faint)
        }
    }
}

/** Downloads per week: organic (light) under ads (solid), bars growing in with a stagger. */
@Composable
fun WeeklyBars(weeks: List<WeekDownloads>, color: Color, key: Any?) {
    val n = weeks.size
    val total = 200 + (n - 1).coerceAtLeast(0) * 90 + 700
    val t = rememberTimeline(key, total)
    val fade = rememberEnter(key, 900, 900, LinearEasing)
    val a = ampli()
    val maxBar: Dp = 100.dp
    val mx = (weeks.maxOfOrNull { it.organic + it.ads } ?: 0L).toFloat().coerceAtLeast(1f) * 1.15f
    Column(Modifier.fillMaxWidth()) {
        Row(
            Modifier.fillMaxWidth().height(maxBar + 30.dp),
            horizontalArrangement = Arrangement.SpaceEvenly,
            verticalAlignment = Alignment.Bottom,
        ) {
            weeks.forEachIndexed { i, wk ->
                val pOrganic = stagePart(t.value, i * 90, 700, EaseOutBack).coerceAtLeast(0f)
                val pAds = stagePart(t.value, 200 + i * 90, 700, EaseOutBack).coerceAtLeast(0f)
                Column(Modifier.width(44.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                    Text(
                        grouped(wk.organic + wk.ads),
                        fontSize = 10.5.sp,
                        fontWeight = FontWeight.Bold,
                        color = a.text,
                        maxLines = 1,
                        modifier = Modifier.graphicsLayer { alpha = fade.value },
                    )
                    Spacer(Modifier.height(4.dp))
                    Box(
                        Modifier
                            .fillMaxWidth()
                            .height(maxBar * (wk.ads.toFloat() / mx) * pAds)
                            .clip(RoundedCornerShape(6.dp))
                            .background(color),
                    )
                    Spacer(Modifier.height(2.dp))
                    Box(
                        Modifier
                            .fillMaxWidth()
                            .height(maxBar * (wk.organic.toFloat() / mx) * pOrganic)
                            .clip(RoundedCornerShape(6.dp))
                            .background(color.copy(alpha = 0.35f)),
                    )
                }
            }
        }
        Row(Modifier.fillMaxWidth().padding(top = 4.dp), horizontalArrangement = Arrangement.SpaceEvenly) {
            weeks.forEachIndexed { i, _ ->
                Text("Wk ${i + 1}", fontSize = 10.sp, color = a.faint, textAlign = TextAlign.Center, modifier = Modifier.width(44.dp))
            }
        }
    }
}

/** A donut whose parts (percent 0..100, colour) sweep in one after another. */
@Composable
fun Donut(parts: List<Pair<Float, Color>>, center: String, sub: String, key: Any?, diameter: Dp = 120.dp) {
    val total = 200 + parts.size * 250 + 1000
    val t = rememberTimeline(key, total)
    val a = ampli()
    Box(Modifier.size(diameter), contentAlignment = Alignment.Center) {
        Canvas(Modifier.fillMaxSize()) {
            val sw = 14.dp.toPx() * (size.minDimension / 120.dp.toPx())
            val inset = sw / 2f
            val arcSize = Size(size.width - sw, size.height - sw)
            drawCircle(a.surface2, radius = (size.minDimension - sw) / 2f, style = Stroke(width = sw))
            var start = -90f
            parts.forEachIndexed { i, part ->
                val sweep = part.first / 100f * 360f
                val p = stagePart(t.value, 200 + i * 250, 1000, RiseEasing)
                if (p > 0f && sweep > 0f) {
                    drawArc(
                        color = part.second,
                        startAngle = start,
                        sweepAngle = sweep * p,
                        useCenter = false,
                        topLeft = Offset(inset, inset),
                        size = arcSize,
                        style = Stroke(width = sw),
                    )
                }
                start += sweep
            }
        }
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            Text(center, fontSize = 22.sp, fontWeight = FontWeight.ExtraBold, maxLines = 1, textAlign = TextAlign.Center)
            Text(sub, fontSize = 10.5.sp, fontWeight = FontWeight.Bold, color = a.muted, maxLines = 1, textAlign = TextAlign.Center)
        }
    }
}

/** A horizontal bar whose fill grows to [fraction] (0..1). */
@Composable
fun AnimatedBar(
    fraction: Float,
    color: Color,
    key: Any?,
    modifier: Modifier = Modifier,
    height: Dp = 8.dp,
    corner: Dp = 99.dp,
    track: Color? = null,
    durationMs: Int = 1100,
) {
    val a = rememberEnter(key, durationMs, 80, RiseEasing)
    val trackColor = track ?: ampli().surface2
    Box(modifier.height(height).clip(RoundedCornerShape(corner)).background(trackColor)) {
        Box(
            Modifier
                .fillMaxHeight()
                .fillMaxWidth((fraction * a.value).coerceIn(0f, 1f))
                .clip(RoundedCornerShape(corner))
                .background(color),
        )
    }
}

/** The positive / mixed / negative bar on the Reviews screen. */
@Composable
fun SentimentGauge(parts: List<Pair<Float, Color>>, key: Any?) {
    val a = rememberEnter(key, 1200, 80, RiseEasing)
    BoxWithConstraints(
        Modifier
            .fillMaxWidth()
            .height(14.dp)
            .clip(RoundedCornerShape(99.dp))
            .background(ampli().surface2),
    ) {
        val full: Dp = maxWidth
        Row(Modifier.fillMaxHeight()) {
            parts.forEach { part ->
                Box(
                    Modifier
                        .fillMaxHeight()
                        .width(full * (part.first / 100f).coerceIn(0f, 1f) * a.value)
                        .background(part.second),
                )
            }
        }
    }
}

/** Lays children out left to right and wraps to a new line when full (for tag chips). */
@Composable
fun Wrap(modifier: Modifier = Modifier, spacing: Dp = 6.dp, content: @Composable () -> Unit) {
    Layout(content = content, modifier = modifier) { measurables, constraints ->
        val gap = spacing.roundToPx()
        val loose = constraints.copy(minWidth = 0, minHeight = 0)
        val placeables = measurables.map { it.measure(loose) }
        val maxW = if (constraints.hasBoundedWidth) constraints.maxWidth else Int.MAX_VALUE
        val xs = IntArray(placeables.size)
        val ys = IntArray(placeables.size)
        var x = 0
        var y = 0
        var rowH = 0
        var widest = 0
        placeables.forEachIndexed { i, p ->
            if (x > 0 && x + p.width > maxW) {
                x = 0
                y += rowH + gap
                rowH = 0
            }
            xs[i] = x
            ys[i] = y
            x += p.width + gap
            if (p.height > rowH) rowH = p.height
            if (x - gap > widest) widest = x - gap
        }
        val width = if (constraints.hasBoundedWidth) constraints.maxWidth else widest
        val height = (if (placeables.isEmpty()) 0 else y + rowH).coerceIn(constraints.minHeight, constraints.maxHeight)
        layout(width, height) {
            placeables.forEachIndexed { i, p -> p.placeRelative(xs[i], ys[i]) }
        }
    }
}

/** The hero's gradient, floating glow and shine sweep, drawn behind the content. */
@Composable
fun Modifier.heroBackground(color: Color): Modifier {
    val inf = rememberInfiniteTransition(label = "hero")
    val float = inf.animateFloat(
        initialValue = 0f,
        targetValue = 1f,
        animationSpec = infiniteRepeatable(tween(3500, easing = FastOutSlowInEasing), RepeatMode.Reverse),
        label = "float",
    )
    val shine = inf.animateFloat(
        initialValue = 0f,
        targetValue = 1f,
        animationSpec = infiniteRepeatable(tween(3600, easing = LinearEasing), RepeatMode.Restart),
        label = "shine",
    )
    val deep = lerp(color, Color.Black, 0.45f)
    return this.drawBehind {
        val w = size.width
        val h = size.height
        drawRect(Brush.linearGradient(listOf(color, deep), start = Offset.Zero, end = Offset(w, h)))
        val f = float.value
        val r = 130.dp.toPx()
        val c = Offset(w - 40.dp.toPx() - 20.dp.toPx() * f, 10.dp.toPx() + 24.dp.toPx() * f)
        drawCircle(
            Brush.radialGradient(0f to Color.White.copy(alpha = 0.28f), 0.65f to Color.Transparent, center = c, radius = r),
            radius = r,
            center = c,
        )
        val s = shine.value
        if (s < 0.6f) {
            val x = -w + 2f * w * (s / 0.6f)
            drawRect(
                Brush.linearGradient(
                    0f to Color.Transparent,
                    0.5f to Color.White.copy(alpha = 0.14f),
                    1f to Color.Transparent,
                    start = Offset(x + 0.3f * w, 0f),
                    end = Offset(x + 0.6f * w, 0.3f * h),
                ),
            )
        }
    }
}
