package com.vikash.ampli.owner.ui

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Checkbox
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import com.vikash.ampli.owner.data.Campaign
import com.vikash.ampli.owner.data.DayStat
import com.vikash.ampli.owner.data.Feature
import com.vikash.ampli.owner.data.Products
import com.vikash.ampli.owner.data.rupees
import java.time.LocalDate

private val prices = mapOf("mp3" to "₹199 Pro, one time", "carhub" to "₹499 Pro (₹299 launch)")
private val roadmapStates = listOf("now" to "Now", "next" to "Next", "later" to "Later")

@Composable
fun ProductsScreen(nav: Nav, modifier: Modifier) {
    Loadable(nav.tick, { nav.repo.features() }, nav::refresh) { all ->
        Page(modifier) {
            Products.names.forEach { (id, name) ->
                val mine = all.filter { it.product == id }
                val built = mine.count { it.state == "built" }
                val left = mine.count { it.state == "left" }
                item(key = id) {
                    OwnerCard(onClick = { nav.go(Route.Product(id)) }) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Dot(productColor(id))
                            Spacer(Modifier.width(10.dp))
                            Text(name, style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
                            Pill("Not on Play", MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                        LinearProgressIndicator(
                            progress = { if (built + left == 0) 0f else built.toFloat() / (built + left) },
                            color = productColor(id),
                            modifier = Modifier.fillMaxWidth().padding(vertical = 10.dp),
                        )
                        Row {
                            Text("$built built · $left left", style = MaterialTheme.typography.bodySmall, modifier = Modifier.weight(1f))
                            Text(prices[id] ?: "", style = MaterialTheme.typography.bodySmall)
                        }
                    }
                }
            }
            item { Section("Roadmap") }
            val road = all.filter { it.state in setOf("now", "next", "later") }
                .sortedBy { f -> roadmapStates.indexOfFirst { it.first == f.state } }
            if (road.isEmpty()) item { Text("Nothing planned yet. Add items from a product page.") }
            items(road, key = { "r${it.id}" }) { f ->
                OwnerCard {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Dot(productColor(f.product))
                        Spacer(Modifier.width(10.dp))
                        Text(f.title, modifier = Modifier.weight(1f))
                        Pill(roadmapStates.first { it.first == f.state }.second, Violet)
                    }
                }
            }
        }
    }
}

@Composable
fun ProductScreen(nav: Nav, product: String, modifier: Modifier) {
    var adding by remember { mutableStateOf(false) }
    Loadable(nav.tick to product, { nav.repo.features().filter { it.product == product } }, nav::refresh) { fs ->
        Page(modifier) {
            item {
                ChoiceRow(Products.names.toList(), product) { nav.back(); nav.go(Route.Product(it)) }
            }
            item { OutlinedButton(onClick = { adding = true }) { Text("Add a feature or plan") } }
            val groups = listOf("left" to "Left to build", "now" to "Now", "next" to "Next", "later" to "Later", "built" to "Built")
            groups.forEach { (state, label) ->
                val list = fs.filter { it.state == state }
                if (list.isNotEmpty()) {
                    item(key = "h$state") { Section("$label · ${list.size}") }
                    items(list, key = { it.id }) { f -> FeatureRow(nav, f) }
                }
            }
        }
    }
    if (adding) AddFeatureDialog(product, onDismiss = { adding = false }) { f ->
        adding = false
        nav.run("Added") { nav.repo.addFeature(f) }
    }
}

@Composable
private fun FeatureRow(nav: Nav, f: Feature) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Checkbox(
            checked = f.state == "built",
            onCheckedChange = { nav.run { nav.repo.setFeatureState(f.id, if (it) "built" else "left") } },
        )
        Text(f.title, modifier = Modifier.weight(1f), color = if (f.state == "built") MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.onSurface)
        if (f.state != "built") TextButton(onClick = {
            // Cycle left -> now -> next -> later -> left, so planning is one tap.
            val next = mapOf("left" to "now", "now" to "next", "next" to "later", "later" to "left")[f.state] ?: "left"
            nav.run { nav.repo.setFeatureState(f.id, next) }
        }) { Text(if (f.state == "left") "Plan" else f.state.replaceFirstChar { it.uppercase() }) }
    }
}

@Composable
private fun AddFeatureDialog(product: String, onDismiss: () -> Unit, onSave: (Feature) -> Unit) {
    var title by remember { mutableStateOf("") }
    var state by remember { mutableStateOf("left") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Add to ${Products.name(product)}") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(title, { title = it }, label = { Text("Feature") }, singleLine = true)
                ChoiceRow(listOf("left" to "To build", "built" to "Built") + roadmapStates, state) { state = it }
            }
        },
        confirmButton = {
            TextButton(enabled = title.isNotBlank(), onClick = { onSave(Feature(product = product, title = title.trim(), state = state)) }) { Text("Add") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

// ---------------- money ----------------

@Composable
fun MoneyScreen(nav: Nav, modifier: Modifier) {
    var days by rememberSaveable { mutableStateOf(30) }
    var addingDay by remember { mutableStateOf(false) }
    var editing by remember { mutableStateOf<Campaign?>(null) }
    val to = LocalDate.now()
    val from = to.minusDays(days - 1L)
    Loadable(nav.tick to days, { nav.repo.stats(from.toString(), to.toString()) to nav.repo.campaigns() }, nav::refresh) { (stats, campaigns) ->
        Page(modifier) {
            item { ChoiceRow(listOf("7" to "7 days", "30" to "30 days", "365" to "1 year"), days.toString()) { days = it.toInt() } }
            item {
                val pro = stats.sumOf { it.proRevenuePaise }
                val ads = stats.sumOf { it.adRevenuePaise }
                OwnerCard {
                    Text("Revenue", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Text(rupees(pro + ads), style = MaterialTheme.typography.headlineMedium, fontWeight = FontWeight.SemiBold)
                    Text("Pro ${rupees(pro)} · Ads ${rupees(ads)}", style = MaterialTheme.typography.bodySmall)
                    RevenueChart(stats, from, days)
                }
            }
            item {
                val mp3 = stats.filter { it.product == "mp3" }.sumOf { it.proSales }
                val car = stats.filter { it.product == "carhub" }.sumOf { it.proSales }
                KpiRow(Triple("MP3 Pro sales", "$mp3", "₹199 each"), Triple("Car Hub Pro sales", "$car", "₹299 to ₹499"))
            }
            item {
                val installs = stats.sumOf { it.installs }
                val sales = stats.sumOf { it.proSales }
                val conv = if (installs == 0) "–" else "%.1f%%".format(sales * 100.0 / installs)
                KpiRow(Triple("Installs", "$installs", "${stats.sumOf { it.uninstalls }} uninstalls"), Triple("Pro conversion", conv, "sales ÷ installs"))
            }
            item { OutlinedButton(onClick = { addingDay = true }) { Text("Enter a day's numbers") } }
            item {
                Text(
                    "Until Play Console and AdMob are connected, type numbers from their dashboards here.",
                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            item { Section("Ads and posts") }
            items(campaigns, key = { it.id }) { c ->
                OwnerCard(onClick = { editing = c }) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Column(Modifier.weight(1f)) {
                            Text(c.name, fontWeight = FontWeight.SemiBold)
                            Text(
                                "${c.platform.replaceFirstChar { it.uppercase() }} · spent ${rupees(c.spendPaise)} of ${rupees(c.budgetPaise)} · ${c.installs} installs",
                                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                        Pill(c.status.replaceFirstChar { it.uppercase() }, if (c.status == "live") Good else Warn)
                    }
                }
            }
            item { OutlinedButton(onClick = { editing = Campaign(platform = "instagram", name = "") }) { Text("Add an ad or post") } }
        }
    }
    if (addingDay) DayDialog(onDismiss = { addingDay = false }) { d ->
        addingDay = false
        nav.run("Saved") { nav.repo.putStat(d) }
    }
    editing?.let { c ->
        CampaignDialog(c, onDismiss = { editing = null }) { saved ->
            editing = null
            nav.run("Saved") { nav.repo.saveCampaign(saved) }
        }
    }
}

@Composable
private fun RevenueChart(stats: List<DayStat>, from: LocalDate, days: Int) {
    val byDay = stats.groupBy { it.day }.mapValues { (_, v) -> v.sumOf { it.proRevenuePaise + it.adRevenuePaise } }
    val values = (0 until days).map { byDay[from.plusDays(it.toLong()).toString()] ?: 0L }
    val max = (values.maxOrNull() ?: 0L).coerceAtLeast(1L)
    val line = MaterialTheme.colorScheme.primary
    val grid = MaterialTheme.colorScheme.outlineVariant
    Canvas(Modifier.fillMaxWidth().height(110.dp).padding(top = 12.dp)) {
        drawLine(grid, Offset(0f, size.height), Offset(size.width, size.height), strokeWidth = 2f)
        if (values.size < 2) return@Canvas
        val step = size.width / (values.size - 1)
        val pts = values.mapIndexed { i, v -> Offset(i * step, size.height - v.toFloat() / max * size.height * 0.9f) }
        val path = Path().apply { pts.forEachIndexed { i, p -> if (i == 0) moveTo(p.x, p.y) else lineTo(p.x, p.y) } }
        val fill = Path().apply { addPath(path); lineTo(size.width, size.height); lineTo(0f, size.height); close() }
        drawPath(fill, line.copy(alpha = 0.12f))
        drawPath(path, line, style = Stroke(width = 5f))
        drawCircle(line, radius = 8f, center = pts.last())
    }
}

@Composable
private fun NumField(label: String, value: String, onChange: (String) -> Unit) {
    OutlinedTextField(
        value, { v -> onChange(v.filter { it.isDigit() || it == '.' }) }, label = { Text(label) }, singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
    )
}

private fun paise(rupees: String) = ((rupees.toDoubleOrNull() ?: 0.0) * 100).toLong()

@Composable
private fun DayDialog(onDismiss: () -> Unit, onSave: (DayStat) -> Unit) {
    var product by remember { mutableStateOf("mp3") }
    var day by remember { mutableStateOf(LocalDate.now().minusDays(1).toString()) }
    var installs by remember { mutableStateOf("") }
    var uninstalls by remember { mutableStateOf("") }
    var sales by remember { mutableStateOf("") }
    var pro by remember { mutableStateOf("") }
    var ads by remember { mutableStateOf("") }
    var rating by remember { mutableStateOf("") }
    val validDay = runCatching { LocalDate.parse(day) }.isSuccess
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("A day's numbers") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                ChoiceRow(Products.names.toList(), product) { product = it }
                OutlinedTextField(day, { day = it }, label = { Text("Day (YYYY-MM-DD)") }, singleLine = true, isError = !validDay)
                NumField("Installs", installs) { installs = it }
                NumField("Uninstalls", uninstalls) { uninstalls = it }
                NumField("Pro sales", sales) { sales = it }
                NumField("Pro revenue ₹ (after Google's cut)", pro) { pro = it }
                NumField("Ad revenue ₹", ads) { ads = it }
                NumField("Rating", rating) { rating = it }
            }
        },
        confirmButton = {
            TextButton(enabled = validDay, onClick = {
                onSave(
                    DayStat(
                        product = product, day = day,
                        installs = installs.toDoubleOrNull()?.toInt() ?: 0, uninstalls = uninstalls.toDoubleOrNull()?.toInt() ?: 0,
                        proSales = sales.toDoubleOrNull()?.toInt() ?: 0, proRevenuePaise = paise(pro), adRevenuePaise = paise(ads),
                        rating = rating.toDoubleOrNull(),
                    ),
                )
            }) { Text("Save") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
private fun CampaignDialog(start: Campaign, onDismiss: () -> Unit, onSave: (Campaign) -> Unit) {
    var c by remember { mutableStateOf(start) }
    var budget by remember { mutableStateOf(if (start.budgetPaise > 0) (start.budgetPaise / 100).toString() else "") }
    var spend by remember { mutableStateOf(if (start.spendPaise > 0) (start.spendPaise / 100).toString() else "") }
    var installs by remember { mutableStateOf(if (start.installs > 0) start.installs.toString() else "") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(if (start.id == 0L) "New ad or post" else "Edit ad or post") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                OutlinedTextField(c.name, { c = c.copy(name = it) }, label = { Text("Name") }, singleLine = true)
                ChoiceRow(listOf("instagram" to "Instagram", "facebook" to "Facebook", "youtube" to "YouTube", "google" to "Google"), c.platform) { c = c.copy(platform = it) }
                ChoiceRow(listOf<Pair<String?, String>>(null to "Company") + Products.names.toList(), c.product) { c = c.copy(product = it) }
                ChoiceRow(listOf("draft" to "Draft", "scheduled" to "Scheduled", "live" to "Live", "ended" to "Ended"), c.status) { c = c.copy(status = it) }
                NumField("Budget ₹", budget) { budget = it }
                NumField("Spent ₹", spend) { spend = it }
                NumField("Installs from it", installs) { installs = it }
                OutlinedTextField(c.postUrl, { c = c.copy(postUrl = it) }, label = { Text("Post link") }, singleLine = true)
            }
        },
        confirmButton = {
            TextButton(enabled = c.name.isNotBlank(), onClick = {
                onSave(c.copy(budgetPaise = paise(budget), spendPaise = paise(spend), installs = installs.toDoubleOrNull()?.toLong() ?: 0))
            }) { Text("Save") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}
