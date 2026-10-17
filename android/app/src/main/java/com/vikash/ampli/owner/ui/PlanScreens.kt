package com.vikash.ampli.owner.ui

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.background
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
import androidx.compose.material.icons.outlined.Visibility
import androidx.compose.material.icons.outlined.VisibilityOff
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.vikash.ampli.owner.data.ChecklistItem
import com.vikash.ampli.owner.data.Integrations
import com.vikash.ampli.owner.data.Products
import com.vikash.ampli.owner.data.RemoteRepo
import com.vikash.ampli.owner.data.Settings
import com.vikash.ampli.owner.data.Sku
import com.vikash.ampli.owner.data.rupees
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch
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
                    Tile("Built vs left", "roadmap for every app", Modifier.weight(1f)) { nav.go(Route.Roadmap) }
                    Tile("Guides", "Play, GST, trademark", Modifier.weight(1f)) { nav.go(Route.Guides) }
                }
            }
            item { MoreFromAmpliCard() }
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
            item {
                AmpliCard {
                    CardTitle("Server", "your own")
                    Text(
                        "Ad account tokens are stored on your server, never on the phone. The app only shows numbers " +
                            "and sends Pause, Resume and Budget actions.",
                        fontSize = 11.5.sp, color = ampli().muted,
                    )
                }
            }
        }
    }
}

/** A preview of the "More from Ampli" list and the chai tip card that every Ampli app shows in Settings. */
@Composable
private fun MoreFromAmpliCard() {
    val a = ampli()
    val taglines = mapOf(
        "mp3" to "Your music, no monthly fee",
        "carhub" to "The car that talks Haryanvi",
        "naukri" to "Free sarkari job alerts",
    )
    AmpliCard {
        CardTitle("In every Ampli app", "Settings › More from Ampli")
        Products.names.forEach { (id, name) ->
            Row(Modifier.fillMaxWidth().padding(vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                ProductBadge(id)
                Spacer(Modifier.width(10.dp))
                Column(Modifier.weight(1f)) {
                    Text(name, fontSize = 13.sp, fontWeight = FontWeight.Bold)
                    Text(taglines[id] ?: "", fontSize = 11.5.sp, color = a.muted)
                }
                PreviewButton("Get", a.surface2, a.text)
            }
        }
        HorizontalDivider(color = a.line, modifier = Modifier.padding(top = 4.dp))
        Row(Modifier.fillMaxWidth().padding(top = 12.dp), verticalAlignment = Alignment.CenterVertically) {
            Box(
                Modifier.size(36.dp).clip(RoundedCornerShape(11.dp)).background(a.warnBg),
                contentAlignment = Alignment.Center,
            ) { Text("☕", fontSize = 17.sp) }
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f)) {
                Text("Buy the developer a chai", fontSize = 13.5.sp, fontWeight = FontWeight.Bold)
                Text("₹49 · ₹99 · ₹199, helps Ampli grow", fontSize = 11.5.sp, color = a.muted)
            }
            PreviewButton("Support", accentFill(), Color.White)
        }
        Text(
            "Owner tracks tips as \"Support\" revenue for each app.",
            fontSize = 11.5.sp, color = a.muted, textAlign = TextAlign.Center,
            modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
        )
    }
}

/** Looks like the in-app button; this is only a preview, so it does nothing here. */
@Composable
private fun PreviewButton(text: String, bg: Color, fg: Color) {
    Text(
        text, color = fg, fontSize = 12.5.sp, fontWeight = FontWeight.ExtraBold, maxLines = 1,
        modifier = Modifier
            .clip(RoundedCornerShape(11.dp))
            .background(bg)
            .padding(horizontal = 12.dp, vertical = 6.dp),
    )
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
    val settings = remember { Settings(context) }
    val scope = rememberCoroutineScope()
    val a = ampli()
    var url by rememberSaveable { mutableStateOf(settings.serverUrl) }
    // Not rememberSaveable: the token must not end up in the saved-state bundle.
    var token by remember { mutableStateOf(settings.token) }
    var showToken by remember { mutableStateOf(false) }
    var testing by remember { mutableStateOf(false) }
    var result by remember { mutableStateOf<Pair<Boolean, String>?>(null) }
    var lock by remember { mutableStateOf(settings.lock) }

    // Coming back from a browser sign-in (AdMob): show the new state.
    OnResume { nav.refresh() }

    val cleanUrl = url.trim().trimEnd('/')
    val urlOk = cleanUrl.startsWith("https://") && cleanUrl.length > "https://".length && !cleanUrl.contains(' ')
    val canTest = urlOk && token.isNotBlank() && !testing

    /** Tests the address and token; saves them when [save] and the test passes. */
    fun test(save: Boolean) {
        testing = true
        result = null
        scope.launch {
            val error: String? = try {
                RemoteRepo.testConnection(cleanUrl, token.trim())
                null
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                e.message ?: "Couldn't reach the server"
            }
            testing = false
            if (error != null) {
                result = false to error
            } else if (save) {
                settings.serverUrl = cleanUrl
                settings.token = token.trim()
                result = true to "Connected and saved"
                onSaved()
            } else {
                result = true to "Connected: the server is up and the token works"
            }
        }
    }

    Page(modifier) {
        item {
            Text(
                if (nav.repo.isRemote) "Connected to your Ampli Owner server." else "Your data is saved on this phone only.",
                fontWeight = FontWeight.SemiBold,
            )
        }
        item {
            Text(
                "Add the server address and owner token to get reports from the apps and keep data in Postgres. " +
                    "Leave both empty to keep working on this phone.",
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        item {
            OutlinedTextField(
                url, { url = it; result = null }, label = { Text("Server address (https://…)") }, singleLine = true,
                isError = url.isNotBlank() && !urlOk,
                supportingText = { if (url.isNotBlank() && !urlOk) Text("Use the full https:// address") },
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
                modifier = Modifier.fillMaxWidth(),
            )
        }
        item {
            OutlinedTextField(
                token, { token = it; result = null }, label = { Text("Owner token") }, singleLine = true,
                visualTransformation = if (showToken) VisualTransformation.None else PasswordVisualTransformation(),
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
                trailingIcon = {
                    IconButton(onClick = { showToken = !showToken }) {
                        Icon(
                            if (showToken) Icons.Outlined.VisibilityOff else Icons.Outlined.Visibility,
                            contentDescription = if (showToken) "Hide token" else "Show token",
                        )
                    }
                },
                modifier = Modifier.fillMaxWidth(),
            )
        }
        item {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                Button(enabled = canTest, onClick = { test(save = true) }) { Text("Test and save") }
                OutlinedButton(enabled = canTest, onClick = { test(save = false) }) { Text("Test") }
            }
        }
        if (testing) item { LinearProgressIndicator(Modifier.fillMaxWidth()) }
        result?.let { (ok, msg) ->
            item { Text(msg, color = if (ok) a.good else a.bad, fontWeight = FontWeight.SemiBold) }
        }
        item {
            TextButton(onClick = {
                url = ""; token = ""; settings.serverUrl = ""; settings.token = ""; onSaved()
            }) { Text("Use this phone only") }
        }
        item {
            AmpliCard {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text("Lock with fingerprint", fontSize = 14.sp, fontWeight = FontWeight.Bold)
                        Text(
                            "Asks for fingerprint, face or the screen lock before showing server data.",
                            fontSize = 11.5.sp, color = a.muted,
                        )
                    }
                    Spacer(Modifier.width(8.dp))
                    Switch(checked = lock, onCheckedChange = { lock = it; settings.lock = it })
                }
            }
        }
        if (nav.repo.isRemote) item { IntegrationsCard(nav) }
        if (nav.repo.isRemote) item { SkusCard(nav) }
    }
}

private val skuKinds = listOf("pro" to "Pro", "tip" to "Tip", "subscription" to "Subscription", "other" to "Other")

/** The price table the server uses to value verified Play purchases (the app never sends amounts). */
@Composable
private fun SkusCard(nav: Nav) {
    val a = ampli()
    var list by remember { mutableStateOf<List<Sku>?>(null) }
    var loadError by remember { mutableStateOf<String?>(null) }
    var editing by remember { mutableStateOf<Sku?>(null) }
    LaunchedEffect(nav.tick) {
        try {
            list = nav.repo.skus()
            loadError = null
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            loadError = e.message ?: "Couldn't load"
        }
    }
    AmpliCard {
        CardTitle("Prices", "Play SKUs")
        Text(
            "Purchases are checked with Google Play and valued from this table, so keep it in step with Play Console.",
            fontSize = 11.5.sp, color = a.muted,
        )
        val skus = list
        when {
            skus == null -> Text(loadError ?: "Loading…", fontSize = 12.sp, color = if (loadError != null) a.bad else a.muted)
            skus.isEmpty() -> Text("No SKUs yet.", fontSize = 12.sp, color = a.muted, modifier = Modifier.padding(top = 8.dp))
            else -> skus.forEachIndexed { i, k ->
                if (i > 0) HorizontalDivider(color = a.line)
                Row(
                    Modifier.fillMaxWidth().clickable { editing = k }.padding(vertical = 9.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Dot(productColor(k.product), 8)
                    Spacer(Modifier.width(8.dp))
                    Column(Modifier.weight(1f)) {
                        Text(k.sku, fontSize = 13.sp, fontWeight = FontWeight.Bold, maxLines = 1)
                        Text(
                            Products.short(k.product) + " · " + (skuKinds.firstOrNull { it.first == k.kind }?.second ?: k.kind),
                            fontSize = 11.sp, color = a.muted,
                        )
                    }
                    Text(rupees(k.pricePaise), fontSize = 13.5.sp, fontWeight = FontWeight.ExtraBold, textAlign = TextAlign.End)
                }
            }
        }
        Row(Modifier.fillMaxWidth().padding(top = 8.dp)) {
            Spacer(Modifier.weight(1f))
            SmallButton("Add SKU", BtnKind.Ghost) { editing = Sku() }
        }
    }
    editing?.let { k -> SkuDialog(nav, k) { editing = null } }
}

@Composable
private fun SkuDialog(nav: Nav, start: Sku, onClose: () -> Unit) {
    val isNew = start.sku.isBlank()
    var product by remember { mutableStateOf(start.product) }
    var sku by remember { mutableStateOf(start.sku) }
    var kind by remember { mutableStateOf(start.kind) }
    var price by remember { mutableStateOf(if (isNew) "" else (start.pricePaise / 100L).toString()) }
    var sending by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    val priceV = price.toLongOrNull()
    val skuOk = sku.isNotBlank() && sku.length <= 100 && sku.all { it.isLetterOrDigit() || it == '_' || it == '.' }
    val ok = skuOk && priceV != null && priceV > 0L
    AlertDialog(
        onDismissRequest = { if (!sending) onClose() },
        title = { Text(if (isNew) "Add SKU" else "Change price") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                if (isNew) {
                    ChoiceRow(Products.names.keys.map { it to Products.short(it) }, product) { product = it }
                    OutlinedTextField(
                        sku, { sku = it.trim().take(100); error = null }, label = { Text("SKU (as in Play Console)") },
                        singleLine = true, modifier = Modifier.fillMaxWidth(),
                    )
                    ChoiceRow(skuKinds, kind) { kind = it }
                } else {
                    Text("${Products.short(product)} · $sku", fontWeight = FontWeight.SemiBold)
                }
                OutlinedTextField(
                    price, { t -> price = t.filter { it.isDigit() }.take(7); error = null },
                    label = { Text("Price ₹ (what the buyer pays)") }, singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = Modifier.fillMaxWidth(),
                )
                error?.let { Text(it, color = ampli().bad, fontSize = 12.sp) }
            }
        },
        confirmButton = {
            TextButton(enabled = ok && !sending, onClick = {
                val next = Sku(product = product, sku = sku.trim(), kind = kind, pricePaise = (priceV ?: 0L) * 100L)
                sending = true
                nav.send(
                    done = "Price saved",
                    onOk = { sending = false; onClose() },
                    onFail = { msg -> sending = false; error = msg },
                ) { nav.repo.putSku(next) }
            }) { Text(if (sending) "Saving…" else "Save") }
        },
        dismissButton = { TextButton(enabled = !sending, onClick = onClose) { Text("Cancel") } },
    )
}

/** AdMob and Play: connected or not, last sync, last error, and a Sync now button. */
@Composable
private fun IntegrationsCard(nav: Nav) {
    val a = ampli()
    var data by remember { mutableStateOf<Integrations?>(null) }
    var loadError by remember { mutableStateOf<String?>(null) }
    LaunchedEffect(nav.tick) {
        try {
            data = nav.repo.integrations()
            loadError = null
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            loadError = e.message ?: "Couldn't load"
        }
    }
    AmpliCard {
        CardTitle("Integrations", "money, vitals, reviews")
        val d = data
        if (d == null) {
            Text(loadError ?: "Loading…", fontSize = 12.sp, color = if (loadError != null) a.bad else a.muted)
        } else {
            IntegrationsBody(nav, d)
        }
    }
}

@Composable
private fun IntegrationsBody(nav: Nav, d: Integrations) {
    val a = ampli()
    val context = LocalContext.current
    var confirmAdmob by remember { mutableStateOf(false) }
    if (confirmAdmob) {
        AlertDialog(
            onDismissRequest = { confirmAdmob = false },
            title = { Text("Disconnect AdMob?") },
            text = { Text("The server forgets the AdMob login and stops pulling ad earnings. Earnings already synced stay.") },
            confirmButton = {
                TextButton(onClick = {
                    confirmAdmob = false
                    nav.run("AdMob disconnected") { nav.repo.disconnectAdmob() }
                }) { Text("Disconnect") }
            },
            dismissButton = { TextButton(onClick = { confirmAdmob = false }) { Text("Cancel") } },
        )
    }
    Column {
        // ---- AdMob ----
        IntegrationRow(
            title = "AdMob",
            sub = when {
                !d.admob.configured -> "The server needs AdMob's app keys first"
                d.admob.connected -> listOfNotNull(
                    d.admob.account.ifBlank { null },
                    d.admob.lastSyncAt?.let { "synced ${ago(it)}" },
                ).joinToString(" · ").ifBlank { "Connected" }
                else -> "Not connected: ad earnings are typed by hand"
            },
            error = d.admob.lastError,
        ) {
            if (d.admob.configured) {
                Column(horizontalAlignment = Alignment.End, verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    SmallButton(if (d.admob.connected) "Reconnect" else "Connect", if (d.admob.connected) BtnKind.Ghost else BtnKind.Primary) {
                        nav.run {
                            val url = nav.repo.connectAdmob()
                            context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
                        }
                    }
                    if (d.admob.connected) {
                        SmallButton("Disconnect", BtnKind.Bad) { confirmAdmob = true }
                    }
                }
            }
        }
        HorizontalDivider(color = a.line)
        // ---- Play ----
        IntegrationRow(
            title = "Google Play",
            sub = if (d.play.configured) {
                "Service account set" + (d.play.lastSyncAt?.let { " · synced ${ago(it)}" } ?: "")
            } else {
                "Add the Play service account to the server"
            },
            error = d.play.lastError,
        ) {
            Chip(
                if (d.play.configured) "● Set" else "Not set",
                if (d.play.configured) a.goodBg else a.surface2,
                if (d.play.configured) a.good else a.muted,
            )
        }
        Row(Modifier.fillMaxWidth().padding(top = 10.dp)) {
            Text(
                "Pulls AdMob earnings, Play vitals, reviews and installs.",
                fontSize = 11.sp, color = a.muted, modifier = Modifier.weight(1f).padding(end = 8.dp),
            )
            SmallButton("Sync now", BtnKind.Primary) {
                nav.run {
                    val r = nav.repo.syncIntegrations()
                    nav.toast(
                        r.errors.firstOrNull()
                            ?: ("Synced ${r.admobDays} AdMob days, ${r.reviews} reviews, ${r.vitals} vitals" +
                                if (r.refunds > 0) ", ${r.refunds} refunds" else ""),
                    )
                }
            }
        }
    }
}

@Composable
private fun IntegrationRow(title: String, sub: String, error: String, action: @Composable () -> Unit) {
    val a = ampli()
    Row(Modifier.fillMaxWidth().padding(vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f)) {
            Text(title, fontSize = 13.5.sp, fontWeight = FontWeight.Bold)
            Text(sub, fontSize = 11.5.sp, color = a.muted)
            if (error.isNotBlank()) Text(error, fontSize = 11.sp, color = a.bad, maxLines = 3)
        }
        Spacer(Modifier.width(8.dp))
        action()
    }
}
