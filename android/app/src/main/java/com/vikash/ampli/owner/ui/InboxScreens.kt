package com.vikash.ampli.owner.ui

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExtendedFloatingActionButton
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.vikash.ampli.owner.data.Products
import com.vikash.ampli.owner.data.Ticket

@Composable
fun InboxScreen(nav: Nav, modifier: Modifier) {
    var filter by rememberSaveable { mutableStateOf<String?>(null) }
    var adding by remember { mutableStateOf(false) }
    Box(modifier.fillMaxSize()) {
        Column {
            Row(
                Modifier.horizontalScroll(rememberScrollState()).padding(horizontal = 16.dp, vertical = 4.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                FilterChip(selected = filter == null, onClick = { filter = null }, label = { Text("All") })
                kindLabel.forEach { (k, l) ->
                    FilterChip(selected = filter == k, onClick = { filter = k }, label = { Text(l) })
                }
            }
            Loadable(nav.tick to filter, { nav.repo.tickets(filter) }, nav::refresh) { list ->
                Page(Modifier) {
                    if (list.isEmpty()) item {
                        Text(
                            "No tickets yet. Reports from the apps land here, and you can log emails or Play reviews with New ticket.",
                            color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(8.dp),
                        )
                    }
                    items(list, key = { it.id }) { t -> TicketRow(t) { nav.go(Route.Ticket(t.id)) } }
                }
            }
        }
        ExtendedFloatingActionButton(
            onClick = { adding = true },
            icon = { Icon(Icons.Filled.Add, contentDescription = null) },
            text = { Text("New ticket") },
            modifier = Modifier.align(Alignment.BottomEnd).padding(16.dp),
        )
    }
    if (adding) NewTicketDialog(onDismiss = { adding = false }) { t ->
        adding = false
        nav.run("Ticket saved") { nav.repo.createTicket(t) }
    }
}

@Composable
private fun TicketRow(t: Ticket, onClick: () -> Unit) {
    OwnerCard(onClick = onClick) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Dot(productColor(t.product))
            Spacer(Modifier.width(10.dp))
            Column(Modifier.weight(1f)) {
                Text(t.title, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                val votes = if (t.kind == "feature" && t.votes > 0) " · ▲ ${t.votes}" else ""
                Text(
                    "${t.ref} · ${statusLabel[t.status] ?: t.status}$votes" + if (t.priority == 1) " · urgent" else "",
                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Pill(kindLabel[t.kind] ?: t.kind, kindColor(t.kind))
        }
    }
}

@Composable
fun TicketScreen(nav: Nav, id: Long, modifier: Modifier) {
    var reply by rememberSaveable(id) { mutableStateOf("") }
    var menu by remember { mutableStateOf(false) }
    Loadable(nav.tick to id, { nav.repo.ticket(id) }, nav::refresh) { d ->
        val t = d.ticket
        Page(modifier) {
            item {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Pill(kindLabel[t.kind] ?: t.kind, kindColor(t.kind))
                    Dot(productColor(t.product))
                    Text(Products.name(t.product), style = MaterialTheme.typography.bodySmall)
                    Text(t.ref, style = MaterialTheme.typography.bodySmall)
                }
            }
            item { Text(t.title, style = MaterialTheme.typography.titleLarge) }
            item {
                val facts = listOfNotNull(
                    t.appVersion.takeIf { it.isNotBlank() }?.let { "Version $it" },
                    t.device.takeIf { it.isNotBlank() }, t.os.takeIf { it.isNotBlank() },
                    t.locale.takeIf { it.isNotBlank() }, if (t.pro) "Pro user" else null,
                    t.orderId.takeIf { it.isNotBlank() }?.let { "Order $it" },
                    t.reporterContact.takeIf { it.isNotBlank() }, "from ${t.source}",
                )
                Text(facts.joinToString(" · "), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            if (t.body.isNotBlank()) item { OwnerCard { Text(t.body) } }
            items(d.messages, key = { it.id }) { m ->
                val mine = m.author == "owner"
                Row(Modifier.fillMaxWidth(), horizontalArrangement = if (mine) Arrangement.End else Arrangement.Start) {
                    Surface(
                        color = if (mine) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.surfaceContainer,
                        shape = MaterialTheme.shapes.large,
                    ) {
                        Text(
                            m.body, modifier = Modifier.padding(12.dp),
                            color = if (mine) MaterialTheme.colorScheme.onPrimary else MaterialTheme.colorScheme.onSurface,
                        )
                    }
                }
            }
            item {
                Box {
                    OutlinedButton(onClick = { menu = true }) { Text("Status: ${statusLabel[t.status] ?: t.status}") }
                    DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                        statusLabel.forEach { (k, l) ->
                            DropdownMenuItem(text = { Text(l) }, onClick = {
                                menu = false
                                nav.run { nav.repo.setTicketStatus(id, k) }
                            })
                        }
                    }
                }
            }
            item {
                OutlinedTextField(
                    value = reply, onValueChange = { reply = it }, modifier = Modifier.fillMaxWidth(),
                    label = { Text(if (t.source == "app") "Reply (shows in the user's app)" else "Note or reply") },
                    minLines = 2,
                    keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Sentences),
                )
            }
            item {
                Button(enabled = reply.isNotBlank(), onClick = {
                    val body = reply.trim()
                    reply = ""
                    nav.run("Sent") { nav.repo.reply(id, body) }
                }) { Text("Send") }
            }
        }
    }
}

@Composable
private fun NewTicketDialog(onDismiss: () -> Unit, onSave: (Ticket) -> Unit) {
    var product by remember { mutableStateOf("mp3") }
    var kind by remember { mutableStateOf("bug") }
    var source by remember { mutableStateOf("email") }
    var title by remember { mutableStateOf("") }
    var body by remember { mutableStateOf("") }
    var who by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("New ticket") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                ChoiceRow(Products.names.toList(), product) { product = it }
                ChoiceRow(kindLabel.toList(), kind) { kind = it }
                ChoiceRow(listOf("email" to "Email", "play" to "Play", "manual" to "Other"), source) { source = it }
                OutlinedTextField(title, { title = it }, label = { Text("Title") }, singleLine = true)
                OutlinedTextField(body, { body = it }, label = { Text("Details") }, minLines = 2)
                OutlinedTextField(who, { who = it }, label = { Text("From (name or email)") }, singleLine = true)
            }
        },
        confirmButton = {
            TextButton(enabled = title.isNotBlank(), onClick = {
                onSave(Ticket(product = product, kind = kind, source = source, title = title.trim(), body = body.trim(), reporterName = who.trim()))
            }) { Text("Save") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

/** A wrapping row of single-choice chips. */
@Composable
fun <K> ChoiceRow(options: List<Pair<K, String>>, selected: K, onPick: (K) -> Unit) {
    Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
        options.forEach { (k, l) -> FilterChip(selected = selected == k, onClick = { onPick(k) }, label = { Text(l) }) }
    }
}

@Composable
fun Divider() = HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
