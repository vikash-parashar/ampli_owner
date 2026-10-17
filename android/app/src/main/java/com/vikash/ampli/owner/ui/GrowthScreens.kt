package com.vikash.ampli.owner.ui

import android.content.Context
import android.content.Intent
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExtendedFloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
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
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.ClipboardManager
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.vikash.ampli.owner.data.Channels
import com.vikash.ampli.owner.data.NewLink
import com.vikash.ampli.owner.data.PROMO_CODE
import com.vikash.ampli.owner.data.Products
import com.vikash.ampli.owner.data.PromoImport
import com.vikash.ampli.owner.data.PromoImportResult
import com.vikash.ampli.owner.data.PromoPair
import com.vikash.ampli.owner.data.TrackedLink
import com.vikash.ampli.owner.data.compactCount
import com.vikash.ampli.owner.data.compactRupees
import com.vikash.ampli.owner.data.grouped
import kotlinx.coroutines.launch

private const val LABEL_MAX = 60

/** A short hint under each channel in the New link sheet. */
private val channelHints = mapOf(
    "play_hindi" to "Your Hindi store page",
    "website" to "Buttons on the site",
    "whatsapp" to "Groups and status",
    "telegram" to "Channels and groups",
    "instagram" to "Bio and posts",
    "youtube" to "Video descriptions",
    "crosspromo" to "Inside another Ampli app",
    "other" to "Anywhere else",
)

private fun TrackedLink.channelTitle(): String = channelName.ifBlank { Channels.name(channel) }

/** "Car Hub Pro → MP3 Pro gift". */
private fun pairTitle(p: PromoPair): String = "${Products.short(p.sourceProduct)} Pro → ${Products.short(p.targetProduct)} Pro gift"

private fun copyLink(clipboard: ClipboardManager, nav: Nav, url: String) {
    clipboard.setText(AnnotatedString(url))
    nav.toast("Link copied")
}

private fun shareLink(context: Context, url: String) {
    val send = Intent(Intent.ACTION_SEND).apply {
        type = "text/plain"
        putExtra(Intent.EXTRA_TEXT, url)
    }
    context.startActivity(Intent.createChooser(send, "Share link"))
}

/** Codes pasted one per line or comma separated, cleaned up: upper case, no blanks, no repeats. */
private fun parseCodes(text: String): List<String> =
    text.split('\n', '\r', ',', ';', ' ', '\t').map { it.trim().uppercase() }.filter { it.isNotEmpty() }.distinct()

private data class GrowthData(val key: String, val links: List<TrackedLink>, val promo: List<PromoPair>)

/** Free installs: tracked links for each channel, and Pro-to-Pro gift codes between the apps. */
@Composable
fun GrowthScreen(nav: Nav, modifier: Modifier) {
    var filter by rememberSaveable { mutableStateOf("all") }
    var creating by remember { mutableStateOf(false) }
    var deleting by remember { mutableStateOf<TrackedLink?>(null) }
    var importing by remember { mutableStateOf<PromoPair?>(null) }

    Box(modifier.fillMaxSize()) {
        Loadable(
            key = nav.tick to filter,
            load = {
                val f = filter
                GrowthData(f, nav.repo.links(if (f == "all") null else f), nav.repo.promo())
            },
            onRetry = nav::refresh,
            keepStale = true,
        ) { d ->
            GrowthContent(
                nav = nav,
                data = d,
                filter = filter,
                onFilter = { filter = it },
                onDelete = { deleting = it },
                onImport = { importing = it },
            )
        }
        val pop = rememberEnter(Unit, 500, 150, EaseOutBack)
        ExtendedFloatingActionButton(
            onClick = { creating = true },
            icon = { Icon(Icons.Filled.Add, contentDescription = null) },
            text = { Text("New link", fontWeight = FontWeight.ExtraBold) },
            containerColor = accentFill(),
            contentColor = Color.White,
            modifier = Modifier
                .align(Alignment.BottomEnd)
                .padding(16.dp)
                .graphicsLayer {
                    val p = pop.value
                    scaleX = 0.6f + 0.4f * p
                    scaleY = 0.6f + 0.4f * p
                    alpha = p.coerceIn(0f, 1f)
                },
        )
        if (creating) {
            NewLinkSheet(nav, startProduct = if (filter == "all") "mp3" else filter) { creating = false }
        }
    }

    deleting?.let { l ->
        AlertDialog(
            onDismissRequest = { deleting = null },
            title = { Text("Delete this link?") },
            text = {
                Text(
                    "${Products.short(l.product)} · ${l.channelTitle()}" + (if (l.label.isNotBlank()) " · ${l.label}" else "") +
                        ". People who tap it still reach Play, but new installs from it won't be counted. This can't be undone.",
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    deleting = null
                    nav.run("Link deleted") { nav.repo.deleteLink(l.id) }
                }) { Text("Delete", color = ampli().bad) }
            },
            dismissButton = { TextButton(onClick = { deleting = null }) { Text("Cancel") } },
        )
    }

    importing?.let { p -> AddCodesDialog(nav, p) { importing = null } }
}

@Composable
private fun GrowthContent(
    nav: Nav,
    data: GrowthData,
    filter: String,
    onFilter: (String) -> Unit,
    onDelete: (TrackedLink) -> Unit,
    onImport: (PromoPair) -> Unit,
) {
    val a = ampli()
    val key = data.key
    val links = data.links
    val installs = links.sumOf { it.installs30d }
    val sales = links.sumOf { it.proSales }
    val net = links.sumOf { it.netPaise }
    ScrollPage(Modifier, key) {
        var n = 0

        ProductSwitcher(selected = filter, onSelect = onFilter, modifier = Modifier.rise(key, n++))

        // ---- hero: free installs from links ----
        HeroCard(Modifier.rise(key, n++)) {
            Text(
                (if (key == "all") "Free installs, all apps" else "Free installs, ${Products.short(key)}") + " · 30 days",
                color = Color.White.copy(alpha = 0.85f), fontSize = 12.sp, fontWeight = FontWeight.Bold,
            )
            CountUpText(
                target = installs.toDouble(), key = key, format = { grouped(it.toLong()) },
                modifier = Modifier.fillMaxWidth().padding(top = 6.dp),
                style = HeroStyle, color = Color.White, textAlign = TextAlign.End,
            )
            Row(Modifier.fillMaxWidth().padding(top = 10.dp)) {
                Spacer(Modifier.weight(1f))
                Chip(
                    if (links.size == 1) "from 1 tracked link" else "from ${links.size} tracked links",
                    Color.White.copy(alpha = 0.2f), Color.White,
                )
            }
        }

        // ---- 2 stat boxes ----
        Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            StatBox("Pro sales from links", Modifier.weight(1f).fillMaxHeight().rise(key, n++)) {
                StatCountUp(sales.toDouble(), key) { compactCount(it.toLong()) }
            }
            StatBox("Earned (net)", Modifier.weight(1f).fillMaxHeight().rise(key, n++), delta = "after Google's fee", deltaColor = a.muted) {
                StatCountUp(net.toDouble(), key) { compactRupees(it.toLong()) }
            }
        }

        // ---- tracked links ----
        AmpliCard(Modifier.rise(key, n++)) {
            CardTitle("Tracked links", if (links.isEmpty()) null else "${links.size}")
            if (links.isEmpty()) {
                Text(
                    "No links yet. Tap New link, pick where you'll share it, and post that link instead of the plain Play link. " +
                        "Installs and Pro sales from it show up here.",
                    fontSize = 13.sp, color = a.muted, textAlign = TextAlign.Center, lineHeight = 18.sp,
                    modifier = Modifier.fillMaxWidth().padding(10.dp),
                )
            }
            links.forEachIndexed { i, l ->
                if (i > 0) HorizontalDivider(color = a.line, modifier = Modifier.padding(vertical = 12.dp))
                LinkItem(nav, l, onDelete)
            }
        }

        // ---- cross-promo ----
        Section("Cross-promo")
        if (data.promo.isEmpty()) {
            AmpliCard(Modifier.rise(key, n++)) {
                CardTitle("Pro buyers get a gift", "no codes yet")
                Text(
                    "Make promo codes in Play Console's Promo codes page and paste them here. Car Hub Pro buyers get one each.",
                    fontSize = 13.sp, color = a.muted, textAlign = TextAlign.Center, lineHeight = 18.sp,
                    modifier = Modifier.fillMaxWidth().padding(10.dp),
                )
                Row(Modifier.fillMaxWidth().padding(top = 4.dp)) {
                    Spacer(Modifier.weight(1f))
                    SmallButton("Add codes", BtnKind.Primary) { onImport(PromoPair()) }
                }
            }
        }
        data.promo.forEach { p -> PromoCard(p, key, Modifier.rise(key, n++)) { onImport(p) } }

        // ---- how it works ----
        AmpliCard(Modifier.rise(key, n++)) {
            CardTitle("How links are counted")
            Text(
                "Each link opens the app's Play page with a referrer. When someone installs and opens the app, " +
                    "it tells the server which link they came from, so installs and Pro sales land on the right channel. " +
                    "Money is net, after Google's fee and GST, from verified purchases only.",
                fontSize = 11.5.sp, color = a.muted, lineHeight = 17.sp,
            )
        }
    }
}

@Composable
private fun LinkItem(nav: Nav, l: TrackedLink, onDelete: (TrackedLink) -> Unit) {
    val a = ampli()
    val context = LocalContext.current
    val clipboard = LocalClipboardManager.current
    Column(Modifier.fillMaxWidth()) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            ProductBadge(l.product, 32.dp)
            Spacer(Modifier.width(10.dp))
            Column(Modifier.weight(1f)) {
                Text(
                    l.channelTitle() + if (l.label.isNotBlank()) " · ${l.label}" else "",
                    fontSize = 13.5.sp, fontWeight = FontWeight.Bold, maxLines = 1, overflow = TextOverflow.Ellipsis,
                )
                val from = if (l.channel == "crosspromo" && l.fromProduct.isNotBlank()) "shown in ${Products.short(l.fromProduct)} · " else ""
                Text(
                    "${Products.short(l.product)} · $from${grouped(l.installs)} installs in all",
                    fontSize = 11.sp, fontWeight = FontWeight.SemiBold, color = a.muted, maxLines = 1, overflow = TextOverflow.Ellipsis,
                )
            }
        }
        Row(
            Modifier.fillMaxWidth().padding(top = 10.dp).height(IntrinsicSize.Min),
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            MiniMetric("Installs, 30 d", compactCount(l.installs30d), Color.Unspecified, Modifier.weight(1f).fillMaxHeight())
            MiniMetric("Pro sales", compactCount(l.proSales), Color.Unspecified, Modifier.weight(1f).fillMaxHeight())
            MiniMetric("Net", compactRupees(l.netPaise), if (l.netPaise > 0) a.good else Color.Unspecified, Modifier.weight(1f).fillMaxHeight())
        }
        if (l.url.isNotBlank()) {
            Text(
                l.url, fontSize = 11.sp, color = a.muted, maxLines = 1, overflow = TextOverflow.Ellipsis,
                modifier = Modifier.padding(top = 8.dp),
            )
        }
        Row(
            Modifier.fillMaxWidth().padding(top = 10.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End),
        ) {
            SmallButton("Delete", BtnKind.Ghost) { onDelete(l) }
            SmallButton("Share", BtnKind.Ghost, enabled = l.url.isNotBlank()) { shareLink(context, l.url) }
            SmallButton("Copy", BtnKind.Primary, enabled = l.url.isNotBlank()) { copyLink(clipboard, nav, l.url) }
        }
    }
}

@Composable
private fun PromoCard(p: PromoPair, key: String, modifier: Modifier, onAdd: () -> Unit) {
    val a = ampli()
    // Codes still owed to buyers who haven't claimed yet; fewer codes than that means someone misses out.
    val owed = (p.eligible - p.claimed).coerceAtLeast(0L)
    val low = p.available < owed || p.available <= 5L
    AmpliCard(modifier) {
        CardTitle(pairTitle(p), p.sku)
        Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            StatBox(
                "Eligible", Modifier.weight(1f).fillMaxHeight(), flat = true,
                delta = "${Products.short(p.sourceProduct)} Pro buyers", deltaColor = a.muted,
            ) { StatCountUp(p.eligible.toDouble(), key) { compactCount(it.toLong()) } }
            StatBox(
                "Claimed", Modifier.weight(1f).fillMaxHeight(), flat = true,
                delta = "codes handed out", deltaColor = a.muted,
            ) { StatCountUp(p.claimed.toDouble(), key) { compactCount(it.toLong()) } }
        }
        Spacer(Modifier.height(10.dp))
        Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            StatBox(
                "Redeemed", Modifier.weight(1f).fillMaxHeight(), flat = true,
                delta = "used in ${Products.short(p.targetProduct)}", deltaColor = a.muted,
            ) { StatCountUp(p.redeemed.toDouble(), key) { compactCount(it.toLong()) } }
            StatBox(
                "Codes left", Modifier.weight(1f).fillMaxHeight(), flat = true,
                delta = if (low) "running low" else "of ${grouped(p.imported)} added",
                deltaColor = if (low) a.warn else a.muted,
            ) { StatCountUp(p.available.toDouble(), key) { compactCount(it.toLong()) } }
        }

        // redeemed / claimed
        val rate = if (p.claimed == 0L) 0.0 else p.redeemed * 100.0 / p.claimed
        Row(Modifier.fillMaxWidth().padding(top = 14.dp), verticalAlignment = Alignment.CenterVertically) {
            Text("Claimed → redeemed", fontSize = 12.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f))
            Text(
                if (p.claimed == 0L) "–" else "${pctText(Math.round(rate * 10) / 10.0)}%",
                fontSize = 12.sp, fontWeight = FontWeight.ExtraBold, textAlign = TextAlign.End,
                color = if (p.claimed == 0L) a.muted else Color.Unspecified,
            )
        }
        AnimatedBar((rate / 100.0).toFloat(), accent(), key, Modifier.fillMaxWidth().padding(top = 6.dp))
        Text(
            if (p.claimed == 0L) {
                "No codes handed out yet."
            } else {
                "${grouped(p.redeemed)} of ${grouped(p.claimed)} people who got a code used it."
            },
            fontSize = 11.5.sp, color = a.muted, textAlign = TextAlign.Center,
            modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
        )
        if (low) {
            Text(
                "Add more codes so every ${Products.short(p.sourceProduct)} Pro buyer gets one.",
                fontSize = 11.5.sp, color = a.warn, fontWeight = FontWeight.SemiBold, textAlign = TextAlign.Center,
                modifier = Modifier.fillMaxWidth().padding(top = 4.dp),
            )
        }
        Row(Modifier.fillMaxWidth().padding(top = 10.dp)) {
            Spacer(Modifier.weight(1f))
            SmallButton("Add codes", BtnKind.Primary) { onAdd() }
        }
    }
}

// ---------------- new link ----------------

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun NewLinkSheet(nav: Nav, startProduct: String, onClose: () -> Unit) {
    val a = ampli()
    val context = LocalContext.current
    val clipboard = LocalClipboardManager.current
    var sending by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    var created by remember { mutableStateOf<TrackedLink?>(null) }
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true, confirmValueChange = { !sending })
    val scope = rememberCoroutineScope()

    var product by remember { mutableStateOf(if (startProduct in Products.names.keys) startProduct else "mp3") }
    var channel by remember { mutableStateOf("whatsapp") }
    var from by remember { mutableStateOf(Products.names.keys.first { it != product }) }
    var label by remember { mutableStateOf("") }
    val cross = channel == "crosspromo"
    val ok = !cross || (from != product && from in Products.names.keys)

    fun close() {
        scope.launch { sheetState.hide() }.invokeOnCompletion { onClose() }
    }

    fun submit() {
        val nl = NewLink(product = product, channel = channel, label = label.trim(), fromProduct = if (cross) from else "")
        sending = true
        error = null
        nav.send(
            done = null,
            onOk = { sending = false },
            onFail = { msg -> sending = false; error = msg },
        ) { created = nav.repo.createLink(nl) }
    }

    ModalBottomSheet(onDismissRequest = { if (!sending) onClose() }, sheetState = sheetState) {
        Column(
            Modifier
                .fillMaxWidth()
                .verticalScroll(rememberScrollState())
                .padding(start = 18.dp, end = 18.dp, bottom = 22.dp),
        ) {
            val made = created
            if (made != null) {
                Text(
                    "Your link is ready", fontSize = 18.sp, fontWeight = FontWeight.ExtraBold,
                    textAlign = TextAlign.Center, modifier = Modifier.fillMaxWidth(),
                )
                Text(
                    "${Products.short(made.product)} · ${made.channelTitle()}" + if (made.label.isNotBlank()) " · ${made.label}" else "",
                    fontSize = 12.5.sp, color = a.muted, textAlign = TextAlign.Center,
                    modifier = Modifier.fillMaxWidth().padding(top = 4.dp),
                )
                Text(
                    made.url, fontSize = 12.5.sp, fontWeight = FontWeight.SemiBold,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 14.dp)
                        .clip(RoundedCornerShape(12.dp))
                        .background(a.surface2)
                        .padding(12.dp),
                )
                WizardNote("Share this link instead of the plain Play link. Installs from it show up on this screen.")
                Row(Modifier.fillMaxWidth().padding(top = 18.dp), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    SmallButton("Share", BtnKind.Ghost, Modifier.weight(1f)) { shareLink(context, made.url) }
                    SmallButton("Copy", BtnKind.Primary, Modifier.weight(1f)) { copyLink(clipboard, nav, made.url) }
                }
                SmallButton("Done", BtnKind.Ghost, Modifier.fillMaxWidth().padding(top = 8.dp)) { close() }
            } else {
                Text(
                    "New link", fontSize = 18.sp, fontWeight = FontWeight.ExtraBold,
                    textAlign = TextAlign.Center, modifier = Modifier.fillMaxWidth(),
                )
                WizardQuestion("Which app?")
                OptionGrid(
                    Products.names.keys.map { id ->
                        WizardOption(Products.short(id), Products.price(id), product == id, !sending) {
                            product = id
                            if (from == id) from = Products.names.keys.first { it != id }
                        }
                    },
                )
                WizardQuestion("Where will you share it?")
                OptionGrid(
                    Channels.names.map { (id, name) ->
                        WizardOption(name, channelHints[id] ?: "", channel == id, !sending) { channel = id }
                    },
                )
                if (cross) {
                    WizardQuestion("Shown in which app?")
                    OptionGrid(
                        Products.names.keys.filter { it != product }.map { id ->
                            WizardOption(Products.short(id), "Settings › More from Ampli", from == id, !sending) { from = id }
                        },
                    )
                }
                Spacer(Modifier.height(12.dp))
                OutlinedTextField(
                    label, { label = it.take(LABEL_MAX) },
                    label = { Text("Name it (optional)") },
                    placeholder = { Text("Family groups") },
                    singleLine = true, enabled = !sending,
                    supportingText = { Text("${label.length} / $LABEL_MAX", textAlign = TextAlign.End, modifier = Modifier.fillMaxWidth()) },
                    modifier = Modifier.fillMaxWidth(),
                )
                WizardNote("Make one link per place you post, so you can see which one brings people in.")
                error?.let {
                    Text(
                        it, color = a.bad, fontSize = 12.5.sp, fontWeight = FontWeight.SemiBold, textAlign = TextAlign.Center,
                        modifier = Modifier.fillMaxWidth().padding(top = 10.dp),
                    )
                }
                Row(Modifier.fillMaxWidth().padding(top = 18.dp), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    SmallButton("Cancel", BtnKind.Ghost, Modifier.weight(1f), enabled = !sending) { close() }
                    SmallButton(
                        if (sending) "Making…" else "Make link", BtnKind.Primary, Modifier.weight(1f), enabled = ok && !sending,
                    ) { submit() }
                }
            }
        }
    }
}

// ---------------- promo codes ----------------

@Composable
private fun AddCodesDialog(nav: Nav, start: PromoPair, onClose: () -> Unit) {
    val a = ampli()
    var source by remember { mutableStateOf(start.sourceProduct) }
    var target by remember { mutableStateOf(start.targetProduct) }
    var sku by remember { mutableStateOf(start.sku) }
    var text by remember { mutableStateOf("") }
    var sending by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    var result by remember { mutableStateOf<PromoImportResult?>(null) }

    val codes = parseCodes(text)
    val good = codes.filter { PROMO_CODE.matches(it) }
    val bad = codes.size - good.size
    val ok = good.isNotEmpty() && source != target && sku.isNotBlank()
    val productOptions = Products.names.keys.map { it to Products.short(it) }

    val done = result
    if (done != null) {
        AlertDialog(
            onDismissRequest = onClose,
            title = { Text("Codes added") },
            text = {
                Column {
                    Text(
                        "${grouped(done.added.toLong())} added" +
                            if (done.skipped + bad > 0) " · ${grouped((done.skipped + bad).toLong())} skipped" else "",
                        fontSize = 20.sp, fontWeight = FontWeight.ExtraBold, textAlign = TextAlign.Center,
                        modifier = Modifier.fillMaxWidth().padding(vertical = 6.dp),
                    )
                    Text(
                        "Skipped codes were already added or didn't look like Play codes. " +
                            "${Products.short(source)} Pro buyers get the new codes from now on.",
                        fontSize = 12.5.sp, color = a.muted, textAlign = TextAlign.Center,
                    )
                }
            },
            confirmButton = { TextButton(onClick = onClose) { Text("Done") } },
        )
        return
    }

    AlertDialog(
        onDismissRequest = { if (!sending) onClose() },
        title = { Text("Add promo codes") },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(
                    "Make promo codes in Play Console's Promo codes page and paste them here, one per line or comma separated.",
                    fontSize = 12.sp, color = a.muted,
                )
                Text("Buyers of", fontSize = 12.sp, fontWeight = FontWeight.Bold, modifier = Modifier.padding(top = 4.dp))
                ChoiceRow(productOptions, source) { source = it; error = null }
                Text("get a gift for", fontSize = 12.sp, fontWeight = FontWeight.Bold)
                ChoiceRow(productOptions, target) { target = it; error = null }
                if (source == target) Text("Pick two different apps.", fontSize = 12.sp, color = a.bad)
                OutlinedTextField(
                    sku, { t -> sku = t.filter { it.isLetterOrDigit() || it == '_' || it == '.' }.take(80); error = null },
                    label = { Text("Play product (SKU)") }, singleLine = true, enabled = !sending,
                    modifier = Modifier.fillMaxWidth(),
                )
                OutlinedTextField(
                    text, { text = it; error = null },
                    label = { Text("Codes") }, minLines = 4, maxLines = 8, enabled = !sending,
                    supportingText = {
                        Text(
                            when {
                                codes.isEmpty() -> "Paste the codes here"
                                bad > 0 -> "${good.size} codes · $bad don't look right (A-Z, 0-9, 6 to 40 long)"
                                else -> "${good.size} codes"
                            },
                        )
                    },
                    isError = bad > 0,
                    modifier = Modifier.fillMaxWidth().heightIn(min = 120.dp),
                )
                error?.let { Text(it, color = a.bad, fontSize = 12.sp) }
            }
        },
        confirmButton = {
            TextButton(enabled = ok && !sending, onClick = {
                val req = PromoImport(sourceProduct = source, targetProduct = target, sku = sku.trim(), codes = good)
                sending = true
                nav.send(
                    done = null,
                    onOk = { sending = false },
                    onFail = { msg -> sending = false; error = msg },
                ) { result = nav.repo.importPromoCodes(req) }
            }) { Text(if (sending) "Adding…" else "Add ${if (good.isEmpty()) "" else grouped(good.size.toLong()) + " "}codes") }
        },
        dismissButton = { TextButton(enabled = !sending, onClick = onClose) { Text("Cancel") } },
    )
}
