package com.vikash.ampli.owner.ui

import android.content.Intent
import android.net.Uri
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.animateDpAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
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
import androidx.compose.foundation.layout.size
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
import androidx.compose.material3.Slider
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.vikash.ampli.owner.data.AdAccount
import com.vikash.ampli.owner.data.AdCampaign
import com.vikash.ampli.owner.data.AdsCampaignsResponse
import com.vikash.ampli.owner.data.AdsPlatform
import com.vikash.ampli.owner.data.Creative
import com.vikash.ampli.owner.data.NewCampaign
import com.vikash.ampli.owner.data.Products
import com.vikash.ampli.owner.data.grouped
import com.vikash.ampli.owner.data.rupees
import kotlinx.coroutines.launch

private data class PlatformLook(val name: String, val sub: String, val bg: Color, val letter: String)

private fun look(id: String): PlatformLook = when (id) {
    "meta" -> PlatformLook("Meta", "Facebook + Instagram", Color(0xFF1877F2), "f")
    "tiktok" -> PlatformLook("TikTok", "TikTok Ads", Color(0xFF111111), "♪")
    "google" -> PlatformLook("Google Ads", "YouTube + Play + Search", Color(0xFFEA4335), "G")
    else -> PlatformLook(id.replaceFirstChar { it.uppercase() }, "", Color(0xFF6B6585), id.take(1).uppercase())
}

private data class Geo(val code: String, val label: String, val cpiRupees: Int)

/** Countries the wizard offers, with a rough cost per install for the estimate. */
private val GEOS = listOf(
    Geo("IN", "🇮🇳 India", 6),
    Geo("US", "🇺🇸 USA", 40),
    Geo("GB", "🇬🇧 UK", 35),
    Geo("AE", "🇦🇪 UAE", 22),
    Geo("CA", "🇨🇦 Canada", 38),
)

private fun countryLabel(code: String): String = GEOS.firstOrNull { it.code == code }?.label ?: code

private fun AdsPlatform.connectedAccounts(): List<AdAccount> = accounts.filter { it.status == "connected" }

private data class AdsData(val platforms: List<AdsPlatform>, val ads: AdsCampaignsResponse)

@Composable
private fun PlatformLogo(id: String, size: Dp = 38.dp) {
    val l = look(id)
    Box(
        Modifier.size(size).clip(RoundedCornerShape(size * 0.32f)).background(l.bg),
        contentAlignment = Alignment.Center,
    ) {
        Text(l.letter, color = Color.White, fontWeight = FontWeight.ExtraBold, fontSize = (size.value * 0.37f).sp, maxLines = 1)
    }
}

@Composable
fun AdsScreen(nav: Nav, modifier: Modifier) {
    val product = nav.product
    val context = LocalContext.current
    var wizard by remember { mutableStateOf(false) }
    var connecting by remember { mutableStateOf<AdsPlatform?>(null) }
    var disconnecting by remember { mutableStateOf<AdAccount?>(null) }

    Box(modifier.fillMaxSize()) {
        Loadable(
            key = nav.tick to product,
            load = { Pair(product, AdsData(nav.repo.adPlatforms(), nav.repo.adCampaigns(product))) },
            onRetry = nav::refresh,
            keepStale = true,
        ) { loaded ->
            AdsContent(
                nav = nav,
                key = loaded.first,
                data = loaded.second,
                onConnect = { connecting = it },
                onDisconnect = { disconnecting = it },
            )
            if (wizard) {
                NewCampaignSheet(nav = nav, platforms = loaded.second.platforms, startProduct = product, onClose = { wizard = false })
            }
        }
        val pop = rememberEnter(Unit, 500, 150, EaseOutBack)
        ExtendedFloatingActionButton(
            onClick = { wizard = true },
            icon = { Icon(Icons.Filled.Add, contentDescription = null) },
            text = { Text("New campaign", fontWeight = FontWeight.ExtraBold) },
            containerColor = accent(),
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
    }

    connecting?.let { p ->
        if (!p.configured) {
            AlertDialog(
                onDismissRequest = { connecting = null },
                title = { Text("${p.name} needs setting up first") },
                text = {
                    Text(
                        "Your Ampli Owner server doesn't have ${p.name}'s developer app keys yet. " +
                            "Create a developer app with ${p.name}, add its app ID and secret to the server's settings, " +
                            "then come back and tap Connect.",
                    )
                },
                confirmButton = { TextButton(onClick = { connecting = null }) { Text("OK") } },
            )
        } else {
            AlertDialog(
                onDismissRequest = { connecting = null },
                icon = { PlatformLogo(p.id, 56.dp) },
                title = { Text("Connect ${p.name}") },
                text = {
                    Text(
                        "You sign in with ${p.name} once. Ampli Owner then reads spend, installs and results, " +
                            "and can create, pause and resume campaigns. The access token stays on your server.",
                    )
                },
                confirmButton = {
                    TextButton(onClick = {
                        connecting = null
                        nav.run {
                            val url = nav.repo.connectAds(p.id)
                            context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
                        }
                    }) { Text("Sign in with ${p.name}") }
                },
                dismissButton = { TextButton(onClick = { connecting = null }) { Text("Cancel") } },
            )
        }
    }

    disconnecting?.let { acc ->
        AlertDialog(
            onDismissRequest = { disconnecting = null },
            title = { Text("Disconnect ${acc.name}?") },
            text = { Text("Ampli Owner stops reading and changing campaigns on this ${look(acc.platform).name} account. Campaigns already running keep running.") },
            confirmButton = {
                TextButton(onClick = {
                    disconnecting = null
                    nav.run("Disconnected") { nav.repo.deleteAdAccount(acc.id) }
                }) { Text("Disconnect") }
            },
            dismissButton = { TextButton(onClick = { disconnecting = null }) { Text("Cancel") } },
        )
    }
}

@Composable
private fun AdsContent(
    nav: Nav,
    key: String,
    data: AdsData,
    onConnect: (AdsPlatform) -> Unit,
    onDisconnect: (AdAccount) -> Unit,
) {
    val a = ampli()
    val s = data.ads.summary
    val campaigns = data.ads.campaigns
    ScrollPage(Modifier, key) {
        var n = 0

        // ---- hero: earned from ads ----
        HeroCard(Modifier.rise(key, n++)) {
            Text(
                (if (key == "all") "Ads for all apps" else "Ads for ${Products.short(key)}") + " · last 30 days",
                color = Color.White.copy(alpha = 0.85f), fontSize = 12.sp, fontWeight = FontWeight.Bold,
            )
            CountUpText(
                target = s.revenuePaise.toDouble(), key = key, format = { rupees(it.toLong()) },
                modifier = Modifier.fillMaxWidth().padding(top = 6.dp),
                style = HeroStyle, color = Color.White, textAlign = TextAlign.End,
            )
            Row(Modifier.fillMaxWidth().padding(top = 10.dp), verticalAlignment = Alignment.Bottom) {
                Chip("earned from ${rupees(s.spendPaise)} spent", Color.White.copy(alpha = 0.2f), Color.White)
                Spacer(Modifier.weight(1f))
                Text(
                    if (s.profitPaise >= 0) "Profit ${rupees(s.profitPaise)}" else "Loss ${rupees(-s.profitPaise)}",
                    color = Color.White, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, maxLines = 1,
                )
            }
        }

        // ---- 4 stat boxes ----
        Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            StatBox("Money invested", Modifier.weight(1f).fillMaxHeight().rise(key, n++)) {
                CountUpText(s.spendPaise.toDouble(), key, { rupees(it.toLong()) }, style = StatStyle, textAlign = TextAlign.End)
            }
            StatBox(
                label = "Return (ROAS)",
                modifier = Modifier.weight(1f).fillMaxHeight().rise(key, n++),
                delta = if (s.roas >= 1.0) "ads pay for themselves" else "losing money",
                deltaColor = if (s.roas >= 1.0) a.good else a.bad,
            ) {
                StatNumber(f2(s.roas) + "x")
            }
        }
        Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            StatBox("Installs from ads", Modifier.weight(1f).fillMaxHeight().rise(key, n++)) {
                CountUpText(s.installs.toDouble(), key, { grouped(it.toLong()) }, style = StatStyle, textAlign = TextAlign.End)
            }
            StatBox("Cost per install", Modifier.weight(1f).fillMaxHeight().rise(key, n++)) {
                StatNumber("₹" + f2(s.cpiPaise / 100.0))
            }
        }

        // ---- ad accounts ----
        AmpliCard(Modifier.rise(key, n++)) {
            CardTitle("Ad accounts", "connected once, run from here")
            data.platforms.forEachIndexed { i, p ->
                if (i > 0) HorizontalDivider(color = a.line)
                val connected = p.connectedAccounts()
                Row(Modifier.fillMaxWidth().padding(vertical = 11.dp), verticalAlignment = Alignment.CenterVertically) {
                    PlatformLogo(p.id)
                    Spacer(Modifier.width(12.dp))
                    Column(Modifier.weight(1f)) {
                        Text(p.name, fontSize = 13.5.sp, fontWeight = FontWeight.Bold)
                        val names = connected.joinToString(", ") { it.name }
                        Text(
                            p.sub + if (names.isNotEmpty()) " · $names" else "",
                            fontSize = 11.5.sp, color = a.muted, maxLines = 2, overflow = TextOverflow.Ellipsis,
                        )
                        p.accounts.filter { it.lastError.isNotBlank() }.forEach { acc ->
                            Text("${acc.name}: ${acc.lastError}", fontSize = 11.sp, color = a.bad, maxLines = 2)
                        }
                    }
                    Spacer(Modifier.width(8.dp))
                    if (connected.isNotEmpty()) {
                        Chip(
                            "● Connected", a.goodBg, a.good,
                            Modifier.clip(RoundedCornerShape(50)).clickable { onDisconnect(connected[0]) },
                        )
                    } else {
                        SmallButton("Connect", BtnKind.Primary) { onConnect(p) }
                    }
                }
            }
        }

        // ---- campaigns ----
        AmpliCard(Modifier.rise(key, n++)) {
            CardTitle("Campaigns", "${campaigns.count { it.status == "live" }} running")
            if (campaigns.isEmpty()) {
                Text(
                    "No campaigns yet. Tap New campaign to start one.",
                    fontSize = 13.sp, color = a.muted, textAlign = TextAlign.Center,
                    modifier = Modifier.fillMaxWidth().padding(10.dp),
                )
            }
            campaigns.forEachIndexed { i, c ->
                if (i > 0) HorizontalDivider(color = a.line, modifier = Modifier.padding(vertical = 12.dp))
                CampaignItem(nav, c)
            }
        }

        // ---- how the verdict works ----
        AmpliCard(Modifier.rise(key, n++)) {
            CardTitle("How the verdict works")
            Text(
                "ROAS = money earned from people who installed through the ad ÷ money spent. " +
                    "1.5x or more says scale, 0.8x to 1.5x says watch, under 0.8x says stop. " +
                    "Under 5 days of data is too early to judge. Installs and purchases are matched to the ad " +
                    "through the Play install referrer.",
                fontSize = 11.5.sp, color = a.muted, lineHeight = 17.sp,
            )
        }
    }
}

@Composable
private fun CampaignItem(nav: Nav, c: AdCampaign) {
    val a = ampli()
    val l = look(c.platform)
    val v = c.verdict
    val (chipBg, chipFg) = when (v.code) {
        "scale" -> a.goodBg to a.good
        "watch", "early" -> a.warnBg to a.warn
        "stop" -> a.badBg to a.bad
        else -> a.surface2 to a.muted
    }
    val statusText = when (c.status) {
        "live" -> "${c.days} days"
        "paused" -> "Paused"
        "in_review" -> "In review"
        "draft" -> "Draft"
        "ended" -> "Ended"
        "error" -> "Error"
        else -> c.status
    }
    val noResults = c.status == "in_review" || c.status == "draft" || c.spendPaise == 0L
    Column(Modifier.fillMaxWidth().alpha(if (c.status == "paused") 0.55f else 1f)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            PlatformLogo(c.platform, 32.dp)
            Spacer(Modifier.width(10.dp))
            Column(Modifier.weight(1f)) {
                Text(c.name, fontSize = 13.5.sp, fontWeight = FontWeight.Bold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text(
                    c.countries.joinToString(", ") { countryLabel(it) } + " · " + statusText,
                    fontSize = 11.sp, fontWeight = FontWeight.SemiBold, color = a.muted, maxLines = 1, overflow = TextOverflow.Ellipsis,
                )
            }
            Spacer(Modifier.width(8.dp))
            Chip(v.label.ifBlank { v.code }, chipBg, chipFg)
        }
        Row(
            Modifier.fillMaxWidth().padding(top = 10.dp).height(IntrinsicSize.Min),
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            MiniMetric("Spent", rupees(c.spendPaise), Color.Unspecified, Modifier.weight(1f).fillMaxHeight())
            MiniMetric("Installs", grouped(c.installs), Color.Unspecified, Modifier.weight(1f).fillMaxHeight())
            MiniMetric("Earned", rupees(c.revenuePaise), Color.Unspecified, Modifier.weight(1f).fillMaxHeight())
            MiniMetric(
                "ROAS",
                if (noResults) "–" else f2(c.roas) + "x",
                if (noResults) Color.Unspecified else if (c.roas >= 1.0) a.good else a.bad,
                Modifier.weight(1f).fillMaxHeight(),
            )
        }
        val why = if (c.status == "error" && c.lastError.isNotBlank()) c.lastError else v.reason
        if (why.isNotBlank()) {
            Text(why, fontSize = 11.5.sp, color = a.muted, lineHeight = 17.sp, modifier = Modifier.padding(top = 8.dp))
        }
        if (c.dailyBudgetPaise > 0) {
            Text(
                "Daily budget ${rupees(c.dailyBudgetPaise)} on ${l.name}",
                fontSize = 11.sp, color = a.faint, modifier = Modifier.padding(top = 4.dp),
            )
        }
        Row(
            Modifier.fillMaxWidth().padding(top = 10.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End),
        ) {
            when (c.status) {
                "paused", "draft" -> SmallButton("Resume", BtnKind.Ok) {
                    nav.run("Resumed on ${l.name}") { nav.repo.resumeCampaign(c.id) }
                }
                "live" -> {
                    SmallButton("Pause", BtnKind.Ghost) {
                        nav.run("Paused on ${l.name}") { nav.repo.pauseCampaign(c.id) }
                    }
                    if (v.code == "scale") {
                        SmallButton("+20% budget", BtnKind.Ok) {
                            val raised = c.dailyBudgetPaise + c.dailyBudgetPaise / 5
                            nav.run("Daily budget raised 20% on ${l.name}") { nav.repo.setBudget(c.id, raised) }
                        }
                    }
                    if (v.code == "stop") {
                        SmallButton("Stop now", BtnKind.Bad) {
                            nav.run("Stopped on ${l.name}") { nav.repo.pauseCampaign(c.id) }
                        }
                    }
                }
                else -> {}
            }
        }
    }
}

/** One of the four small boxes in a campaign: label top-left, value bottom-right. */
@Composable
private fun MiniMetric(label: String, value: String, valueColor: Color, modifier: Modifier) {
    val a = ampli()
    Column(
        modifier
            .clip(RoundedCornerShape(11.dp))
            .background(a.surface2)
            .heightIn(min = 50.dp)
            .padding(horizontal = 8.dp, vertical = 7.dp),
        verticalArrangement = Arrangement.SpaceBetween,
    ) {
        Text(label, fontSize = 10.sp, fontWeight = FontWeight.Bold, color = a.muted, textAlign = TextAlign.Start, maxLines = 1)
        Text(
            value, fontSize = 13.sp, fontWeight = FontWeight.ExtraBold, color = valueColor,
            textAlign = TextAlign.End, maxLines = 1, overflow = TextOverflow.Ellipsis,
            modifier = Modifier.fillMaxWidth(),
        )
    }
}

// ---------------- new campaign wizard ----------------

private val stepNames = listOf("Which app?", "Where?", "Which countries?", "Budget", "Creative", "Review")

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun NewCampaignSheet(nav: Nav, platforms: List<AdsPlatform>, startProduct: String, onClose: () -> Unit) {
    val a = ampli()
    val accentColor = accent()
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    val scope = rememberCoroutineScope()

    val firstConnected: AdsPlatform? = platforms.firstOrNull { it.connectedAccounts().isNotEmpty() }
    var step by remember { mutableIntStateOf(0) }
    var product by remember { mutableStateOf(if (startProduct == "all") "mp3" else startProduct) }
    var platformId by remember { mutableStateOf<String?>(firstConnected?.id) }
    var accountId by remember { mutableStateOf<Long?>(firstConnected?.connectedAccounts()?.firstOrNull()?.id) }
    var countries by remember { mutableStateOf<Set<String>>(setOf("IN")) }
    var budget by remember { mutableFloatStateOf(1000f) }
    var days by remember { mutableFloatStateOf(7f) }
    var kind by remember { mutableStateOf("video") }
    var url by remember { mutableStateOf("") }
    var headline by remember { mutableStateOf("") }
    var body by remember { mutableStateOf("") }

    fun close() {
        scope.launch { sheetState.hide() }.invokeOnCompletion { onClose() }
    }

    val platform: AdsPlatform? = platforms.firstOrNull { it.id == platformId }
    val platformName = platform?.name ?: (platformId?.let { look(it).name } ?: "")
    val budgetRupees = budget.toLong()
    val dayCount = days.toInt()
    val totalRupees = budgetRupees * dayCount
    val chosenGeos = GEOS.filter { it.code in countries }
    val avgCpi = if (chosenGeos.isEmpty()) 6.0 else chosenGeos.sumOf { it.cpiRupees }.toDouble() / chosenGeos.size
    val expectedInstalls = Math.round(totalRupees / avgCpi)

    val canNext = when (step) {
        1 -> platformId != null && accountId != null
        2 -> countries.isNotEmpty()
        4 -> url.isNotBlank()
        else -> true
    }

    ModalBottomSheet(onDismissRequest = onClose, sheetState = sheetState) {
        Column(
            Modifier
                .fillMaxWidth()
                .verticalScroll(rememberScrollState())
                .padding(start = 18.dp, end = 18.dp, bottom = 22.dp),
        ) {
            Text(
                stepNames[step], fontSize = 18.sp, fontWeight = FontWeight.ExtraBold,
                textAlign = TextAlign.Center, modifier = Modifier.fillMaxWidth(),
            )
            // step dots: the current one is wider, done ones take the accent
            Row(
                Modifier.fillMaxWidth().padding(top = 10.dp, bottom = 14.dp),
                horizontalArrangement = Arrangement.Center,
            ) {
                stepNames.indices.forEach { i ->
                    val w by animateDpAsState(if (i == step) 34.dp else 22.dp, tween(300), label = "stepWidth")
                    val c by animateColorAsState(if (i <= step) accentColor else a.line, tween(300), label = "stepColor")
                    Box(
                        Modifier
                            .padding(horizontal = 2.5.dp)
                            .width(w)
                            .height(5.dp)
                            .clip(RoundedCornerShape(9.dp))
                            .background(c),
                    )
                }
            }

            when (step) {
                0 -> OptionGrid(
                    Products.names.keys.map { id ->
                        WizardOption(Products.short(id), Products.price(id), product == id, true) { product = id }
                    },
                )
                1 -> {
                    OptionGrid(
                        platforms.map { p ->
                            val accs = p.connectedAccounts()
                            WizardOption(
                                title = p.name,
                                sub = p.sub + if (accs.isEmpty()) " · connect first" else "",
                                selected = platformId == p.id,
                                enabled = accs.isNotEmpty(),
                            ) {
                                platformId = p.id
                                accountId = accs.firstOrNull()?.id
                            }
                        },
                    )
                    val accs = platform?.connectedAccounts() ?: emptyList()
                    if (accs.size > 1) {
                        WizardNote("Which ${platformName} account?")
                        ChoiceRow(accs.map { it.id to it.name }, accountId ?: 0L) { accountId = it }
                    }
                    if (platforms.none { it.connectedAccounts().isNotEmpty() }) {
                        WizardNote("Connect an ad account in the Ads tab first.")
                    }
                }
                2 -> OptionGrid(
                    GEOS.map { g ->
                        WizardOption(g.label, "CPI about ₹${g.cpiRupees}", g.code in countries, true) {
                            countries = if (g.code in countries) countries - g.code else countries + g.code
                        }
                    },
                )
                3 -> {
                    WizardQuestion("Daily budget")
                    BigValue(rupees(budgetRupees * 100))
                    Slider(
                        value = budget,
                        onValueChange = { budget = (Math.round(it / 100f) * 100).toFloat().coerceIn(200f, 10000f) },
                        valueRange = 200f..10000f,
                        steps = 97,
                    )
                    WizardQuestion("For how many days")
                    BigValue("$dayCount days")
                    Slider(
                        value = days,
                        onValueChange = { days = Math.round(it).toFloat().coerceIn(3f, 30f) },
                        valueRange = 3f..30f,
                        steps = 26,
                    )
                    WizardNote("Total ${rupees(totalRupees * 100)}. You can pause any time from the Ads tab.")
                }
                4 -> {
                    ChoiceRow(listOf("video" to "Video", "image" to "Image"), kind) { kind = it }
                    Spacer(Modifier.height(8.dp))
                    OutlinedTextField(
                        url, { url = it }, label = { Text(if (kind == "video") "Video link (https://…)" else "Image link (https://…)") },
                        singleLine = true, modifier = Modifier.fillMaxWidth(),
                    )
                    OutlinedTextField(headline, { headline = it }, label = { Text("Headline") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                    OutlinedTextField(body, { body = it }, label = { Text("Text") }, minLines = 2, modifier = Modifier.fillMaxWidth())
                    WizardNote("Use a public link to the file. 9:16 video works for Reels, TikTok and Shorts.")
                }
                else -> {
                    val accountName = platform?.accounts?.firstOrNull { it.id == accountId }?.name ?: ""
                    ReviewRow("App", Products.name(product))
                    ReviewRow("Platform", if (accountName.isNotEmpty()) "$platformName · $accountName" else platformName)
                    ReviewRow("Countries", chosenGeos.joinToString(", ") { it.label })
                    ReviewRow("Budget", "${rupees(budgetRupees * 100)} × $dayCount days")
                    ReviewRow("Total", rupees(totalRupees * 100))
                    ReviewRow("Creative", (if (kind == "video") "Video" else "Image") + if (headline.isNotBlank()) " · ${headline.trim()}" else "")
                    ReviewRow("Expected installs", "about ${grouped(expectedInstalls)}")
                    WizardNote("Owner creates the campaign on $platformName and tracks installs and purchases with a UTM link.")
                }
            }

            Row(Modifier.fillMaxWidth().padding(top = 18.dp), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                SmallButton(if (step == 0) "Cancel" else "Back", BtnKind.Ghost, Modifier.weight(1f)) {
                    if (step == 0) close() else step -= 1
                }
                SmallButton(
                    if (step == stepNames.lastIndex) "Launch campaign" else "Next",
                    BtnKind.Primary, Modifier.weight(1f), enabled = canNext,
                ) {
                    if (step < stepNames.lastIndex) {
                        step += 1
                    } else {
                        val acc = accountId
                        if (acc != null) {
                            val name = "${Products.short(product)} · $platformName · " + chosenGeos.joinToString(" + ") { it.code }
                            val nc = NewCampaign(
                                product = product,
                                adAccountId = acc,
                                name = name,
                                countries = chosenGeos.map { it.code },
                                dailyBudgetPaise = budgetRupees * 100,
                                days = dayCount,
                                creative = Creative(kind = kind, url = url.trim(), thumbnailUrl = "", headline = headline.trim(), text = body.trim()),
                                launch = true,
                            )
                            nav.run("Sent to $platformName for approval") { nav.repo.createCampaign(nc) }
                            close()
                        }
                    }
                }
            }
        }
    }
}

private class WizardOption(
    val title: String,
    val sub: String,
    val selected: Boolean,
    val enabled: Boolean,
    val onClick: () -> Unit,
)

/** Two-column grid of option tiles (the preview's .opts). */
@Composable
private fun OptionGrid(options: List<WizardOption>) {
    Column(verticalArrangement = Arrangement.spacedBy(9.dp)) {
        options.chunked(2).forEach { pair ->
            Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min), horizontalArrangement = Arrangement.spacedBy(9.dp)) {
                pair.forEach { o -> OptionTile(o, Modifier.weight(1f).fillMaxHeight()) }
                if (pair.size == 1) Spacer(Modifier.weight(1f))
            }
        }
    }
}

@Composable
private fun OptionTile(o: WizardOption, modifier: Modifier) {
    val a = ampli()
    val accentColor = accent()
    val border by animateColorAsState(if (o.selected) accentColor else a.line, tween(200), label = "optBorder")
    Surface(
        onClick = o.onClick,
        enabled = o.enabled,
        modifier = modifier.alpha(if (o.enabled) 1f else 0.5f),
        shape = RoundedCornerShape(15.dp),
        color = if (o.selected) accentColor.copy(alpha = 0.10f) else Color.Transparent,
        border = BorderStroke(2.dp, border),
    ) {
        Column(Modifier.padding(12.dp)) {
            Text(o.title, fontSize = 13.5.sp, fontWeight = FontWeight.Bold, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Text(o.sub, fontSize = 11.sp, color = a.muted)
        }
    }
}

@Composable
private fun WizardQuestion(text: String) {
    Text(
        text, fontSize = 12.5.sp, fontWeight = FontWeight.Bold, color = ampli().muted, textAlign = TextAlign.Center,
        modifier = Modifier.fillMaxWidth().padding(top = 14.dp, bottom = 8.dp),
    )
}

@Composable
private fun BigValue(text: String) {
    Text(
        text, fontSize = 30.sp, fontWeight = FontWeight.ExtraBold, textAlign = TextAlign.Center,
        modifier = Modifier.fillMaxWidth().padding(vertical = 6.dp),
    )
}

@Composable
private fun WizardNote(text: String) {
    Text(
        text, fontSize = 11.5.sp, color = ampli().muted, textAlign = TextAlign.Center, lineHeight = 17.sp,
        modifier = Modifier.fillMaxWidth().padding(top = 10.dp),
    )
}

@Composable
private fun ReviewRow(label: String, value: String) {
    Row(Modifier.fillMaxWidth().padding(vertical = 4.dp), verticalAlignment = Alignment.Top) {
        Text(label, fontSize = 13.sp, fontWeight = FontWeight.SemiBold, color = ampli().muted)
        Spacer(Modifier.width(14.dp))
        Text(value, fontSize = 13.sp, fontWeight = FontWeight.Bold, textAlign = TextAlign.End, modifier = Modifier.weight(1f))
    }
}
