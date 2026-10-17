package com.vikash.ampli.owner.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
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
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import com.vikash.ampli.owner.data.DayStat
import com.vikash.ampli.owner.data.Feature
import com.vikash.ampli.owner.data.Products
import java.time.LocalDate

private val prices = mapOf("mp3" to "₹199 Pro, one time", "carhub" to "₹499 Pro (₹299 launch)", "naukri" to "Free, leads + ads")
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

// ---------------- numbers by hand (until Play sync exists) ----------------

@Composable
private fun NumField(label: String, value: String, onChange: (String) -> Unit) {
    OutlinedTextField(
        value, { v -> onChange(v.filter { it.isDigit() || it == '.' }) }, label = { Text(label) }, singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
    )
}

private fun paise(rupees: String) = ((rupees.toDoubleOrNull() ?: 0.0) * 100).toLong()

/** Enter one day's installs and money for a product (opened from the Dashboard). */
@Composable
fun DayDialog(onDismiss: () -> Unit, onSave: (DayStat) -> Unit) {
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
