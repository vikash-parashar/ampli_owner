package com.vikash.ampli.owner.ui

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.vikash.ampli.owner.data.ChecklistItem
import com.vikash.ampli.owner.data.Products
import com.vikash.ampli.owner.data.Settings
import java.time.LocalDate

@Composable
fun PlanScreen(nav: Nav, modifier: Modifier) {
    Loadable(nav.tick, { nav.repo.overview() }, nav::refresh) { o ->
        Page(modifier) {
            item {
                Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    Tile("Launch checklist", "${o.checklistDone}/${o.checklistTotal} done", Modifier.weight(1f)) { nav.go(Route.Checklist) }
                    Tile("To-do", "${o.openTodos.size} open", Modifier.weight(1f)) { nav.go(Route.Todos) }
                }
            }
            item {
                Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    Tile("Built vs left", "both products", Modifier.weight(1f)) { nav.tab = Tab.Products }
                    Tile("Guides", "Play, GST, trademark", Modifier.weight(1f)) { nav.go(Route.Guides) }
                }
            }
            item { Section("Business decisions") }
            item {
                OwnerCard {
                    listOf(
                        "Company type" to "Sole proprietor + Udyam first, Pvt Ltd later",
                        "Bank" to "Current account named Ampli, for Play payouts",
                        "Trademark" to "\"Ampli\" in classes 9 and 42, search first",
                        "Accountant" to "A CA for GST, TDS and returns",
                        "Car Hub break-even" to "About 500 Pro sales",
                    ).forEach { (k, v) ->
                        Column(Modifier.padding(vertical = 6.dp)) {
                            Text(k, fontWeight = FontWeight.SemiBold)
                            Text(v, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun Tile(title: String, sub: String, modifier: Modifier, onClick: () -> Unit) {
    OwnerCard(modifier, onClick) {
        Text(title, style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.SemiBold)
        Text(sub, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

private val nextStatus = mapOf("todo" to "in-progress", "in-progress" to "done", "done" to "todo")

@Composable
fun ChecklistScreen(nav: Nav, modifier: Modifier) {
    var phase by rememberSaveable { mutableStateOf<String?>(null) }
    val context = LocalContext.current
    Loadable(nav.tick, { nav.repo.checklist() }, nav::refresh) { items ->
        val phases = items.map { it.phase }.distinct()
        Page(modifier) {
            item {
                val done = items.count { it.status == "done" }
                OwnerCard {
                    Text("$done done · ${items.count { it.status == "in-progress" }} in progress · ${items.size - done} left", fontWeight = FontWeight.SemiBold)
                    LinearProgressIndicator(
                        progress = { if (items.isEmpty()) 0f else done.toFloat() / items.size },
                        modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
                    )
                    Text(
                        "Tap the box to move an item: to do, in progress, done.",
                        style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(top = 6.dp),
                    )
                }
            }
            item { ChoiceRow(listOf<Pair<String?, String>>(null to "All") + phases.map { it to it }, phase) { phase = it } }
            phases.filter { phase == null || it == phase }.forEach { p ->
                val list = items.filter { it.phase == p }
                item(key = "h$p") { Section("$p · ${list.count { it.status == "done" }}/${list.size}") }
                items(list, key = { it.id }) { c ->
                    ChecklistRow(c, onToggle = { nav.run { nav.repo.setChecklistStatus(c.id, nextStatus[c.status] ?: "todo") } }) {
                        if (c.link.isNotBlank()) runCatching { context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(c.link))) }
                    }
                }
            }
        }
    }
}

@Composable
private fun ChecklistRow(c: ChecklistItem, onToggle: () -> Unit, onOpen: () -> Unit) {
    Row(Modifier.fillMaxWidth().clickable(onClick = onOpen).padding(vertical = 6.dp), verticalAlignment = Alignment.Top) {
        val color = when (c.status) {
            "done" -> Good
            "in-progress" -> Warn
            else -> MaterialTheme.colorScheme.outlineVariant
        }
        Surface(
            onClick = onToggle,
            shape = RoundedCornerShape(7.dp),
            color = if (c.status == "done") Good else Color.Transparent,
            border = androidx.compose.foundation.BorderStroke(2.dp, color),
            modifier = Modifier.size(26.dp),
        ) {
            Box(contentAlignment = Alignment.Center) {
                if (c.status == "done") Icon(Icons.Filled.Check, contentDescription = "Done", tint = Color.White, modifier = Modifier.size(18.dp))
                if (c.status == "in-progress") Text("…", color = Warn)
            }
        }
        Spacer(Modifier.width(12.dp))
        Column(Modifier.weight(1f)) {
            Text(c.title, fontWeight = FontWeight.Medium)
            if (c.detail.isNotBlank()) Text(c.detail, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            val tags = c.products.joinToString(", ") + if (c.link.isNotBlank()) " · tap for the official page" else ""
            if (tags.isNotBlank()) Text(tags, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.primary)
        }
    }
}

@Composable
fun TodosScreen(nav: Nav, modifier: Modifier) {
    var title by rememberSaveable { mutableStateOf("") }
    var product by rememberSaveable { mutableStateOf<String?>(null) }
    var due by rememberSaveable { mutableStateOf<String?>(null) }
    Loadable(nav.tick, { nav.repo.todos() }, nav::refresh) { todos ->
        Page(modifier) {
            item {
                OwnerCard {
                    OutlinedTextField(title, { title = it }, label = { Text("New to-do") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                    ChoiceRow(listOf<Pair<String?, String>>(null to "Company") + Products.names.toList(), product) { product = it }
                    val today = LocalDate.now()
                    ChoiceRow(
                        listOf<Pair<String?, String>>(null to "No date", today.toString() to "Today", today.plusDays(1).toString() to "Tomorrow", today.plusDays(7).toString() to "Next week"),
                        due,
                    ) { due = it }
                    Button(enabled = title.isNotBlank(), onClick = {
                        val t = title.trim()
                        val p = product
                        val d = due
                        title = ""
                        nav.run("Added") { nav.repo.addTodo(t, d, p) }
                    }) { Text("Add") }
                }
            }
            items(todos, key = { it.id }) { t ->
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Checkbox(checked = t.done, onCheckedChange = { nav.run { nav.repo.setTodoDone(t.id, it) } })
                    Dot(productColor(t.product), 8)
                    Spacer(Modifier.width(8.dp))
                    Column(Modifier.weight(1f)) {
                        Text(t.title, color = if (t.done) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.onSurface)
                        t.due?.let { Text(it, style = MaterialTheme.typography.bodySmall) }
                    }
                    IconButton(onClick = { nav.run { nav.repo.deleteTodo(t.id) } }) {
                        Icon(Icons.Outlined.Delete, contentDescription = "Delete")
                    }
                }
            }
        }
    }
}

@Composable
fun GuidesScreen(nav: Nav, modifier: Modifier) {
    Loadable(nav.tick, { nav.repo.guides() }, nav::refresh) { guides ->
        Page(modifier) {
            items(guides, key = { it.id }) { g ->
                OwnerCard(onClick = { nav.go(Route.Guide(g.id)) }) { Text(g.title, fontWeight = FontWeight.SemiBold) }
            }
        }
    }
}

@Composable
fun GuideScreen(nav: Nav, id: String, modifier: Modifier) {
    Loadable(nav.tick to id, { nav.repo.guides().first { it.id == id } }, nav::refresh) { g ->
        Page(modifier) {
            item { Text(g.title, style = MaterialTheme.typography.titleLarge) }
            // Light markdown: headings, bullets and paragraphs. Tables and links stay as text.
            g.bodyMd.split("\n").filter { it.isNotBlank() }.forEach { line ->
                item {
                    val l = line.trim()
                    when {
                        l.startsWith("#") -> Text(l.trimStart('#', ' '), style = MaterialTheme.typography.titleMedium, modifier = Modifier.padding(top = 8.dp))
                        l.startsWith("- ") || l.startsWith("* ") -> Text("•  " + clean(l.drop(2)))
                        else -> Text(clean(l))
                    }
                }
            }
        }
    }
}

private fun clean(s: String) = s.replace("**", "").replace("`", "")

@Composable
fun SettingsScreen(nav: Nav, modifier: Modifier, onSaved: () -> Unit) {
    val context = LocalContext.current
    val settings = androidx.compose.runtime.remember { Settings(context) }
    var url by rememberSaveable { mutableStateOf(settings.serverUrl) }
    var token by rememberSaveable { mutableStateOf(settings.token) }
    Page(modifier) {
        item {
            Text(
                if (nav.repo.isRemote) "Connected to your Ampli Owner server." else "Your data is saved on this phone only.",
                fontWeight = FontWeight.SemiBold,
            )
        }
        item {
            Text(
                "Add the server address and owner token to get reports from the apps and keep data in Postgres. Leave both empty to keep working on this phone.",
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        item { OutlinedTextField(url, { url = it }, label = { Text("Server address (https://…)") }, singleLine = true, modifier = Modifier.fillMaxWidth()) }
        item { OutlinedTextField(token, { token = it }, label = { Text("Owner token") }, singleLine = true, modifier = Modifier.fillMaxWidth()) }
        item {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = { settings.serverUrl = url; settings.token = token; onSaved() }) { Text("Save") }
                TextButton(onClick = { url = ""; token = ""; settings.serverUrl = ""; settings.token = ""; onSaved() }) { Text("Use this phone only") }
            }
        }
    }
}
