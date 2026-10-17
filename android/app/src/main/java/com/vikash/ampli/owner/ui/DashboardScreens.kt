package com.vikash.ampli.owner.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.vikash.ampli.owner.data.Alert
import com.vikash.ampli.owner.data.Dashboard
import com.vikash.ampli.owner.data.Products
import com.vikash.ampli.owner.data.Widget
import com.vikash.ampli.owner.data.WidgetRow
import com.vikash.ampli.owner.data.compactCount
import com.vikash.ampli.owner.data.compactRupees
import com.vikash.ampli.owner.data.rupees
import kotlinx.coroutines.CancellationException

/** The number style of the stat boxes (24sp, extra bold, tight). */
val StatStyle = TextStyle(fontSize = 24.sp, fontWeight = FontWeight.ExtraBold, letterSpacing = (-0.6).sp)

/** The big hero number. */
val HeroStyle = TextStyle(fontSize = 34.sp, fontWeight = FontWeight.ExtraBold, letterSpacing = (-1).sp)

/** 64 -> "64", 10.3 -> "10.3". */
fun pctText(p: Double): String = if (p == Math.floor(p)) Math.round(p).toString() else f1(p)

@Composable
fun DashboardScreen(nav: Nav, modifier: Modifier) {
    val product = nav.product
    var addingDay by remember { mutableStateOf(false) }
    Loadable(
        key = nav.tick to product,
        load = {
            val d = nav.repo.dashboard(product)
            // Alerts are extra: an older server without /v1/alerts still shows the dashboard.
            val alerts = try {
                nav.repo.alerts()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                emptyList()
            }
            DashData(product, d, alerts)
        },
        onRetry = nav::refresh,
        keepStale = true,
    ) { loaded ->
        DashboardContent(nav, loaded.key, loaded.dashboard, loaded.alerts, modifier) { addingDay = true }
    }
    if (addingDay) DayDialog(startProduct = product, onDismiss = { addingDay = false }) { d ->
        addingDay = false
        nav.run("Saved") { nav.repo.putStat(d) }
    }
}

private class DashData(val key: String, val dashboard: Dashboard, val alerts: List<Alert>)

@Composable
private fun DashboardContent(nav: Nav, key: String, d: Dashboard, alerts: List<Alert>, modifier: Modifier, onAddDay: () -> Unit) {
    val a = ampli()
    val accent = accent()
    // Typing a day's numbers only makes sense against the server; phone-only mode shows sample numbers.
    val canType = nav.repo.isRemote
    ScrollPage(modifier, key) {
        var n = 0

        // ---- nothing synced or typed yet ----
        if (d.businessPaise == 0L && d.downloads == 0L && d.reviews == 0L) {
            AmpliCard(Modifier.rise(key, n++)) {
                CardTitle("No data yet")
                Text(
                    "Connect Play and AdMob in Settings" + (if (canType) ", or type a day's numbers." else "."),
                    fontSize = 12.5.sp, color = a.muted, lineHeight = 18.sp,
                )
                Row(
                    Modifier.fillMaxWidth().padding(top = 10.dp),
                    horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End),
                ) {
                    SmallButton("Open Settings", BtnKind.Ghost) { nav.open("open_settings") }
                    if (canType) SmallButton("Add day's numbers", BtnKind.Primary) { onAddDay() }
                }
            }
        }

        // ---- needs your attention ----
        val mine = alerts.filter { key == "all" || it.product == key }
        if (mine.isNotEmpty()) {
            AmpliCard(Modifier.rise(key, n++)) {
                CardTitle("Needs your attention", "${mine.size}")
                mine.forEachIndexed { i, al ->
                    if (i > 0) HorizontalDivider(color = a.line)
                    AlertRow(al, showProduct = key == "all") { nav.open(al.action) }
                }
            }
        }

        // ---- hero: business so far ----
        HeroCard(Modifier.rise(key, n++)) {
            Text(
                (if (key == "all") "All Ampli apps" else Products.name(key)) + " · business so far",
                color = Color.White.copy(alpha = 0.85f), fontSize = 12.sp, fontWeight = FontWeight.Bold,
                textAlign = TextAlign.Start,
            )
            CountUpText(
                target = d.businessPaise.toDouble(), key = key, format = { rupees(it.toLong()) },
                modifier = Modifier.fillMaxWidth().padding(top = 6.dp),
                style = HeroStyle, color = Color.White, textAlign = TextAlign.End,
            )
            Row(Modifier.fillMaxWidth().padding(top = 10.dp), verticalAlignment = Alignment.Bottom) {
                Chip(changeText(d.businessChangePct) + " vs last month", Color.White.copy(alpha = 0.2f), Color.White)
                Spacer(Modifier.weight(1f))
                Text(
                    if (key == "all") (if (d.products.isNotEmpty()) "${d.products.size} apps live" else "All apps") else Products.price(key),
                    color = Color.White, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, maxLines = 1,
                    overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(start = 8.dp),
                )
            }
        }

        // ---- 2x2 stat boxes: label top-left, number bottom-right ----
        Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            StatBox(
                label = "Play downloads",
                modifier = Modifier.weight(1f).fillMaxHeight().rise(key, n++),
                delta = "+${compactCount(d.downloadsWeek)} this week", deltaColor = a.good,
                spark = d.weeklyDownloads.map { (it.organic + it.ads).toFloat() }, sparkKey = key,
            ) {
                StatCountUp(d.downloads.toDouble(), key) { compactCount(it.toLong()) }
            }
            StatBox(
                label = "Rating",
                modifier = Modifier.weight(1f).fillMaxHeight().rise(key, n++),
                delta = if (d.reviews == 0L) "no reviews yet" else "${compactCount(d.reviews)} reviews",
                deltaColor = a.muted,
            ) {
                val r = d.rating
                if (r == null) {
                    StatNumber("–", a.muted)
                } else {
                    Row {
                        CountUpText(r, key, { f1(it) }, modifier = Modifier.alignByBaseline(), style = StatStyle, textAlign = TextAlign.End)
                        Text(" ★", color = Star, fontSize = 20.sp, fontWeight = FontWeight.Bold, modifier = Modifier.alignByBaseline())
                    }
                }
            }
        }
        Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            StatBox(
                label = "Active users",
                modifier = Modifier.weight(1f).fillMaxHeight().rise(key, n++),
                delta = changeText(d.activeChangePct), deltaColor = if (d.activeChangePct >= 0) a.good else a.bad,
            ) {
                StatCountUp(d.activeUsers.toDouble(), key) { compactCount(it.toLong()) }
            }
            when (key) {
                "all" -> {
                    val noSpend = d.ads.spendPaise == 0L
                    StatBox(
                        label = "Ad spend ROAS",
                        modifier = Modifier.weight(1f).fillMaxHeight().rise(key, n++),
                        delta = if (noSpend) "no spend yet" else "${compactRupees(d.ads.revenuePaise)} back",
                        deltaColor = if (noSpend) a.muted else if (d.ads.roas >= 1.0) a.good else a.bad,
                    ) {
                        if (noSpend) StatNumber("–", a.muted) else StatNumber(f2(d.ads.roas) + "x")
                    }
                }
                "naukri" -> StatBox(
                    label = "Other income",
                    modifier = Modifier.weight(1f).fillMaxHeight().rise(key, n++),
                    delta = "+ ${compactRupees(d.adRevenuePaise)} AdMob", deltaColor = a.good,
                ) {
                    StatCountUp(d.otherRevenuePaise.toDouble(), key) { compactRupees(it.toLong()) }
                }
                else -> StatBox(
                    label = "Pro sales",
                    modifier = Modifier.weight(1f).fillMaxHeight().rise(key, n++),
                    delta = compactRupees(d.proRevenuePaise), deltaColor = a.good,
                ) {
                    StatCountUp(d.proSales.toDouble(), key) { compactCount(it.toLong()) }
                }
            }
        }

        // ---- Play vitals (crash and ANR rate vs Play's bad-behaviour limits) ----
        d.vitals?.let { v ->
            AmpliCard(Modifier.rise(key, n++)) {
                CardTitle("Play vitals", if (v.day.isNotBlank()) v.day else null)
                Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    VitalBox("Crash rate", v.crashRate, CRASH_LIMIT, Modifier.weight(1f).fillMaxHeight())
                    VitalBox("ANR rate", v.anrRate, ANR_LIMIT, Modifier.weight(1f).fillMaxHeight())
                }
            }
        }

        // ---- revenue line ----
        AmpliCard(Modifier.rise(key, n++)) {
            CardTitle("Revenue, last 30 days", rupees(d.revenue30d.sum()))
            RevenueLineChart(d.revenue30d, accent, key)
        }

        // ---- weekly downloads ----
        AmpliCard(Modifier.rise(key, n++)) {
            CardTitle("Downloads per week", "Play Console")
            WeeklyBars(d.weeklyDownloads, accent, key)
            Legend(listOf("Organic" to accent.copy(alpha = 0.35f), "From ads" to accent))
        }

        // ---- ratings ----
        AmpliCard(Modifier.rise(key, n++)) {
            CardTitle("Ratings and reviews", "Play Store")
            val s = d.sentiment
            if (d.reviews == 0L && d.stars.sum() == 0L) {
                Text(
                    "No reviews yet", fontSize = 13.sp, color = a.muted, textAlign = TextAlign.Center,
                    modifier = Modifier.fillMaxWidth().padding(vertical = 14.dp),
                )
            } else {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Donut(
                        parts = listOf(s.positive.toFloat() to a.good, s.mixed.toFloat() to a.warn, s.negative.toFloat() to a.bad),
                        center = "${Math.round(s.positive)}%",
                        sub = "positive",
                        key = key,
                    )
                    Spacer(Modifier.width(14.dp))
                    Column(Modifier.weight(1f)) {
                        val totalStars = d.stars.sum().coerceAtLeast(1L)
                        d.stars.take(5).forEachIndexed { i, v ->
                            val barColor = if (i < 2) a.good else if (i == 2) a.warn else a.bad
                            Row(Modifier.padding(vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
                                Text("${5 - i}★", modifier = Modifier.width(24.dp), fontSize = 12.sp, fontWeight = FontWeight.Bold)
                                AnimatedBar(v.toFloat() / totalStars.toFloat(), barColor, key, Modifier.weight(1f))
                                Text(
                                    compactCount(v), modifier = Modifier.width(44.dp), fontSize = 12.sp, fontWeight = FontWeight.Bold,
                                    color = a.muted, textAlign = TextAlign.End, maxLines = 1,
                                )
                            }
                        }
                    }
                }
                Legend(
                    listOf(
                        "Positive ${Math.round(s.positive)}%" to a.good,
                        "Mixed ${Math.round(s.mixed)}%" to a.warn,
                        "Negative ${Math.round(s.negative)}%" to a.bad,
                    ),
                )
            }
        }

        // ---- all apps: your apps list ----
        if (key == "all" && d.products.isNotEmpty()) {
            AmpliCard(Modifier.rise(key, n++)) {
                CardTitle("Your apps", "tap to open")
                d.products.forEachIndexed { i, p ->
                    if (i > 0) HorizontalDivider(color = a.line)
                    Row(
                        Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(12.dp))
                            .clickable { nav.product = p.id }
                            .padding(vertical = 10.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        ProductBadge(p.id)
                        Spacer(Modifier.width(12.dp))
                        Column(Modifier.weight(1f)) {
                            Text(p.name, fontSize = 14.sp, fontWeight = FontWeight.ExtraBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                            Text(
                                "${compactCount(p.downloads)} downloads · ${p.rating?.let { f1(it) } ?: "–"}★",
                                fontSize = 11.5.sp, color = a.muted, maxLines = 1, overflow = TextOverflow.Ellipsis,
                            )
                        }
                        Column(horizontalAlignment = Alignment.End) {
                            Text(compactRupees(p.businessPaise), fontSize = 15.sp, fontWeight = FontWeight.ExtraBold, textAlign = TextAlign.End, maxLines = 1)
                            Text(
                                changeText(p.changePct), fontSize = 11.sp, fontWeight = FontWeight.Bold,
                                color = if (p.changePct >= 0) a.good else a.bad, textAlign = TextAlign.End,
                            )
                        }
                    }
                }
            }
        }

        // ---- one app: where its money comes from (built here from the four revenue lines) ----
        if (key != "all" && d.widgets.none { it.id == "money" }) {
            moneySplit(d)?.let { w -> WidgetCard(w, key, Modifier.rise(key, n++)) }
        }

        // ---- product widgets (funnel, bars, split) ----
        d.widgets.forEach { w ->
            WidgetCard(w, key, Modifier.rise(key, n++))
        }

        // ---- numbers by hand, for what isn't synced yet ----
        if (canType) {
            AmpliCard(Modifier.rise(key, n++)) {
                CardTitle("Numbers by hand", "what isn't synced")
                Text(
                    "For anything Play and AdMob don't send yet, type a day's installs, users and money from their dashboards.",
                    fontSize = 12.sp, color = a.muted,
                )
                Spacer(Modifier.height(10.dp))
                Row(Modifier.fillMaxWidth()) {
                    Spacer(Modifier.weight(1f))
                    SmallButton("Add day's numbers", BtnKind.Primary) { onAddDay() }
                }
            }
        }
    }
}

/** Play's bad-behaviour limits (fractions): above them Play shows the app to fewer people. */
private const val CRASH_LIMIT = 0.0109
private const val ANR_LIMIT = 0.0047

/** A stat number that counts up and shrinks for long values. */
@Composable
fun StatCountUp(target: Double, key: Any?, format: (Double) -> String) {
    val size = statSize(format(target))
    CountUpText(target, key, format, style = StatStyle.copy(fontSize = size), textAlign = TextAlign.End)
}

@Composable
private fun VitalBox(label: String, rate: Double?, limit: Double, modifier: Modifier) {
    val a = ampli()
    val color = when {
        rate == null -> a.muted
        rate >= limit -> a.bad
        rate >= limit * 0.75 -> a.warn
        else -> a.good
    }
    Column(
        modifier
            .clip(RoundedCornerShape(12.dp))
            .background(a.surface2)
            .padding(horizontal = 12.dp, vertical = 10.dp),
    ) {
        Text(label, fontSize = 11.sp, fontWeight = FontWeight.Bold, color = a.muted, maxLines = 1)
        Text(
            if (rate == null) "–" else f2(rate * 100) + "%",
            fontSize = 20.sp, fontWeight = FontWeight.ExtraBold, color = color, textAlign = TextAlign.End, maxLines = 1,
            modifier = Modifier.fillMaxWidth().padding(top = 4.dp),
        )
        Text(
            "limit " + f2(limit * 100) + "%", fontSize = 10.5.sp, color = a.muted, textAlign = TextAlign.End, maxLines = 1,
            modifier = Modifier.fillMaxWidth(),
        )
    }
}

@Composable
private fun AlertRow(al: Alert, showProduct: Boolean, onClick: () -> Unit) {
    val a = ampli()
    val dot = when (al.severity) {
        "high" -> a.bad
        "medium" -> a.warn
        else -> a.muted
    }
    Row(
        Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(10.dp))
            .clickable(enabled = al.action.isNotBlank(), onClick = onClick)
            .padding(vertical = 10.dp),
        verticalAlignment = Alignment.Top,
    ) {
        Box(Modifier.padding(top = 5.dp)) { Dot(dot, 9) }
        Spacer(Modifier.width(10.dp))
        Column(Modifier.weight(1f)) {
            val prefix = if (showProduct && al.product.isNotBlank()) Products.short(al.product) + " · " else ""
            Text(prefix + al.title, fontSize = 13.sp, fontWeight = FontWeight.Bold, lineHeight = 18.sp)
            if (al.detail.isNotBlank()) {
                Text(al.detail, fontSize = 11.5.sp, color = a.muted, lineHeight = 16.sp, maxLines = 3, overflow = TextOverflow.Ellipsis)
            }
        }
        if (al.action.isNotBlank()) {
            Text("›", fontSize = 20.sp, color = a.faint, modifier = Modifier.padding(start = 8.dp))
        }
    }
}

/** The "Money split" donut for one app, from pro / ads / tips / other revenue; null when there's no money yet. */
private fun moneySplit(d: Dashboard): Widget? {
    val parts = listOf(
        "Pro sales" to d.proRevenuePaise,
        "Ads (AdMob)" to d.adRevenuePaise,
        "Tips" to d.tipsPaise,
        "Other" to d.otherRevenuePaise,
    ).filter { it.second > 0L }
    val total = parts.sumOf { it.second }
    if (total <= 0L) return null
    return Widget(
        id = "money", title = "Money split", kind = "split",
        rows = parts.map { (label, paise) -> WidgetRow(label, paise.toDouble(), paise * 100.0 / total) },
    )
}

@Composable
private fun WidgetCard(w: Widget, key: String, modifier: Modifier) {
    val aside = when (w.id) {
        "funnel" -> "30 days"
        "money" -> "all time"
        else -> null
    }
    AmpliCard(modifier) {
        CardTitle(w.title, aside)
        when (w.kind) {
            "funnel" -> FunnelWidget(w.rows, key)
            "split" -> SplitWidget(w, key)
            else -> BarsWidget(w.rows, key)
        }
    }
}

@Composable
private fun FunnelWidget(rows: List<WidgetRow>, key: String) {
    val color = accent()
    rows.forEach { r ->
        Column(Modifier.padding(vertical = 5.dp)) {
            Row(Modifier.fillMaxWidth()) {
                Text(r.label, fontSize = 12.sp, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text("${compactCount(r.value.toLong())} · ${pctText(r.pct)}%", fontSize = 12.sp, fontWeight = FontWeight.Bold, textAlign = TextAlign.End)
            }
            Spacer(Modifier.height(4.dp))
            if (r.value > 0.0) AnimatedBar(
                fraction = (maxOf(r.pct, 6.0) / 100.0).toFloat(),
                color = color.copy(alpha = 0.9f),
                key = key,
                modifier = Modifier.fillMaxWidth(),
                height = 24.dp,
                corner = 8.dp,
                track = Color.Transparent,
                durationMs = 1000,
            )
        }
    }
}

@Composable
private fun BarsWidget(rows: List<WidgetRow>, key: String) {
    val a = ampli()
    val color = accent()
    val maxPct = rows.maxOfOrNull { it.pct } ?: 0.0
    val scale = if (maxPct <= 0.0) 1.0 else maxPct
    rows.forEach { r ->
        Row(Modifier.fillMaxWidth().padding(vertical = 5.dp), verticalAlignment = Alignment.CenterVertically) {
            Text(
                r.label, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f),
            )
            Spacer(Modifier.width(8.dp))
            AnimatedBar((r.pct / scale).toFloat(), color, key, Modifier.width(110.dp))
            val right = if (r.value == r.pct) "${pctText(r.pct)}%" else compactCount(r.value.toLong())
            Text(
                right, fontSize = 12.sp, fontWeight = FontWeight.Bold, color = a.muted, textAlign = TextAlign.End, maxLines = 1,
                modifier = Modifier.widthIn(min = 44.dp).padding(start = 8.dp),
            )
        }
    }
}

/** Colour for each row of a split: the product's colour when the row is a product, otherwise accent then palette. */
@Composable
private fun splitColors(rows: List<WidgetRow>): List<Color> {
    val a = ampli()
    val accent = accent()
    val pool = listOf(accent, a.violet, a.rose, a.cyan, a.saffron).distinct()
    var next = 0
    return rows.map { r ->
        val label = r.label.lowercase()
        val productId = Products.names.keys.firstOrNull { id ->
            label == Products.short(id).lowercase() || label == Products.name(id).lowercase()
        }
        if (productId != null) {
            a.product(productId)
        } else {
            val c = pool[next % pool.size]
            next++
            c
        }
    }
}

@Composable
private fun SplitWidget(w: Widget, key: String) {
    val a = ampli()
    val colors = splitColors(w.rows)
    val money = w.id == "money"
    val first = w.rows.firstOrNull()
    val center = if (key == "all" && money) "${w.rows.size}" else first?.let { "${Math.round(it.pct)}%" } ?: ""
    val sub = if (key == "all" && money) "apps" else first?.label?.substringBefore(" (") ?: ""
    Row(verticalAlignment = Alignment.CenterVertically) {
        Donut(
            parts = w.rows.mapIndexed { i, r -> r.pct.toFloat() to colors[i] },
            center = center,
            sub = sub,
            key = key,
        )
        Spacer(Modifier.width(14.dp))
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            w.rows.forEachIndexed { i, r ->
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Dot(colors[i], 8)
                    Spacer(Modifier.width(6.dp))
                    Text(
                        r.label, fontSize = 13.sp, fontWeight = FontWeight.SemiBold, color = a.muted,
                        maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f),
                    )
                    Text(
                        if (money) compactRupees(r.value.toLong()) else "${pctText(r.pct)}%",
                        fontSize = 13.sp, fontWeight = FontWeight.ExtraBold, textAlign = TextAlign.End, maxLines = 1,
                    )
                }
            }
        }
    }
}
