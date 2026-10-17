package com.vikash.ampli.owner.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
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
import com.vikash.ampli.owner.data.Dashboard
import com.vikash.ampli.owner.data.Products
import com.vikash.ampli.owner.data.Widget
import com.vikash.ampli.owner.data.WidgetRow
import com.vikash.ampli.owner.data.grouped
import com.vikash.ampli.owner.data.rupees

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
        load = { Pair(product, nav.repo.dashboard(product)) },
        onRetry = nav::refresh,
        keepStale = true,
    ) { loaded ->
        DashboardContent(nav, loaded.first, loaded.second, modifier) { addingDay = true }
    }
    if (addingDay) DayDialog(onDismiss = { addingDay = false }) { d ->
        addingDay = false
        nav.run("Saved") { nav.repo.putStat(d) }
    }
}

@Composable
private fun DashboardContent(nav: Nav, key: String, d: Dashboard, modifier: Modifier, onAddDay: () -> Unit) {
    val a = ampli()
    val accent = accent()
    ScrollPage(modifier, key) {
        var n = 0

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
                )
            }
        }

        // ---- 2x2 stat boxes: label top-left, number bottom-right ----
        Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            StatBox(
                label = "Play downloads",
                modifier = Modifier.weight(1f).fillMaxHeight().rise(key, n++),
                delta = "+${grouped(d.downloadsWeek)} this week", deltaColor = a.good,
                spark = d.weeklyDownloads.map { (it.organic + it.ads).toFloat() }, sparkKey = key,
            ) {
                CountUpText(d.downloads.toDouble(), key, { grouped(it.toLong()) }, style = StatStyle, textAlign = TextAlign.End)
            }
            StatBox(
                label = "Rating",
                modifier = Modifier.weight(1f).fillMaxHeight().rise(key, n++),
                delta = "${grouped(d.reviews)} reviews", deltaColor = a.muted,
            ) {
                val r = d.rating
                if (r == null) {
                    StatNumber("–")
                } else {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        CountUpText(r, key, { f1(it) }, style = StatStyle, textAlign = TextAlign.End)
                        Text(" ★", color = Star, fontSize = 20.sp, fontWeight = FontWeight.Bold)
                    }
                }
            }
        }
        Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            StatBox(
                label = "Active users (30 d)",
                modifier = Modifier.weight(1f).fillMaxHeight().rise(key, n++),
                delta = changeText(d.activeChangePct), deltaColor = if (d.activeChangePct >= 0) a.good else a.bad,
                spark = d.revenue30d.takeLast(10).map { it.toFloat() }, sparkKey = key,
            ) {
                CountUpText(d.activeUsers.toDouble(), key, { grouped(it.toLong()) }, style = StatStyle, textAlign = TextAlign.End)
            }
            when (key) {
                "all" -> StatBox(
                    label = "Ad spend ROAS",
                    modifier = Modifier.weight(1f).fillMaxHeight().rise(key, n++),
                    delta = "${rupees(d.ads.revenuePaise)} back", deltaColor = if (d.ads.roas >= 1.0) a.good else a.bad,
                ) {
                    StatNumber(f2(d.ads.roas) + "x")
                }
                "naukri" -> StatBox(
                    label = "Coaching leads",
                    modifier = Modifier.weight(1f).fillMaxHeight().rise(key, n++),
                    delta = "+ ${rupees(d.adRevenuePaise)} AdMob", deltaColor = a.good,
                ) {
                    CountUpText(d.otherRevenuePaise.toDouble(), key, { rupees(it.toLong()) }, style = StatStyle, textAlign = TextAlign.End)
                }
                else -> StatBox(
                    label = "Pro sales",
                    modifier = Modifier.weight(1f).fillMaxHeight().rise(key, n++),
                    delta = rupees(d.proRevenuePaise), deltaColor = a.good,
                ) {
                    CountUpText(d.proSales.toDouble(), key, { grouped(it.toLong()) }, style = StatStyle, textAlign = TextAlign.End)
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
                                grouped(v), modifier = Modifier.width(44.dp), fontSize = 12.sp, fontWeight = FontWeight.Bold,
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
                                "${grouped(p.downloads)} downloads · ${p.rating?.let { f1(it) } ?: "–"}★",
                                fontSize = 11.5.sp, color = a.muted, maxLines = 1,
                            )
                        }
                        Column(horizontalAlignment = Alignment.End) {
                            Text(rupees(p.businessPaise), fontSize = 15.sp, fontWeight = FontWeight.ExtraBold, textAlign = TextAlign.End)
                            Text(
                                changeText(p.changePct), fontSize = 11.sp, fontWeight = FontWeight.Bold,
                                color = if (p.changePct >= 0) a.good else a.bad, textAlign = TextAlign.End,
                            )
                        }
                    }
                }
            }
        }

        // ---- product widgets (funnel, bars, split) ----
        d.widgets.forEach { w ->
            WidgetCard(w, key, Modifier.rise(key, n++))
        }

        // ---- numbers by hand until Play sync exists ----
        AmpliCard(Modifier.rise(key, n++)) {
            CardTitle("Numbers by hand", "until Play sync")
            Text(
                "Until Play Console and AdMob are connected, type a day's installs and money from their dashboards.",
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
                Text("${grouped(r.value.toLong())} · ${pctText(r.pct)}%", fontSize = 12.sp, fontWeight = FontWeight.Bold, textAlign = TextAlign.End)
            }
            Spacer(Modifier.height(4.dp))
            AnimatedBar(
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
            val right = if (r.value == r.pct) "${pctText(r.pct)}%" else grouped(r.value.toLong())
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
                        if (money) rupees(r.value.toLong()) else "${pctText(r.pct)}%",
                        fontSize = 13.sp, fontWeight = FontWeight.ExtraBold, textAlign = TextAlign.End, maxLines = 1,
                    )
                }
            }
        }
    }
}
