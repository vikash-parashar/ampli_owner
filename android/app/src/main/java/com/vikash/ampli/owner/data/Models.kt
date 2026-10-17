package com.vikash.ampli.owner.data

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

// Field names match the Go server's JSON (server/internal/store).

@Serializable
data class Ticket(
    val id: Long = 0,
    val ref: String = "",
    val product: String = "mp3",
    val kind: String = "bug",
    val title: String = "",
    val body: String = "",
    val status: String = "new",
    val priority: Int = 2,
    val source: String = "manual",
    @SerialName("reporter_name") val reporterName: String = "",
    @SerialName("reporter_contact") val reporterContact: String = "",
    @SerialName("app_version") val appVersion: String = "",
    val device: String = "",
    val os: String = "",
    val locale: String = "",
    val pro: Boolean = false,
    @SerialName("order_id") val orderId: String = "",
    val votes: Int = 0,
    @SerialName("github_issue_url") val githubIssueUrl: String = "",
    @SerialName("created_at") val createdAt: String = "",
    @SerialName("updated_at") val updatedAt: String = "",
)

@Serializable
data class Message(
    val id: Long = 0,
    @SerialName("ticket_id") val ticketId: Long = 0,
    val author: String = "owner",
    val body: String = "",
    @SerialName("created_at") val createdAt: String = "",
)

@Serializable
data class TicketDetail(val ticket: Ticket, val messages: List<Message> = emptyList())

@Serializable
data class ChecklistItem(
    val id: String,
    val phase: String,
    val title: String,
    val detail: String = "",
    val products: List<String> = emptyList(),
    val status: String = "todo",
    val link: String = "",
    val order: Int = 0,
    @SerialName("done_at") val doneAt: String? = null,
)

@Serializable
data class Todo(
    val id: Long = 0,
    val title: String,
    val due: String? = null,
    val product: String? = null,
    val done: Boolean = false,
    @SerialName("created_at") val createdAt: String = "",
)

@Serializable
data class Feature(
    val id: Long = 0,
    val product: String,
    val title: String,
    val state: String, // built | left | now | next | later
    @SerialName("ticket_id") val ticketId: Long? = null,
    val build: String = "",
    val order: Int = 0,
)

@Serializable
data class DayStat(
    val product: String,
    val day: String,
    val installs: Int = 0,
    val uninstalls: Int = 0,
    @SerialName("active_users") val activeUsers: Int = 0,
    val rating: Double? = null,
    val reviews: Int = 0,
    @SerialName("pro_sales") val proSales: Int = 0,
    @SerialName("pro_revenue_paise") val proRevenuePaise: Long = 0,
    @SerialName("ad_revenue_paise") val adRevenuePaise: Long = 0,
    @SerialName("tips_paise") val tipsPaise: Long = 0,
    @SerialName("other_revenue_paise") val otherRevenuePaise: Long = 0,
)

@Serializable
data class Guide(val id: String, val title: String, @SerialName("body_md") val bodyMd: String = "")

@Serializable
data class Overview(
    @SerialName("open_tickets") val openTickets: Int = 0,
    @SerialName("waiting_on_owner") val waitingOnOwner: Int = 0,
    @SerialName("tickets_by_kind") val ticketsByKind: Map<String, Int> = emptyMap(),
    @SerialName("checklist_total") val checklistTotal: Int = 0,
    @SerialName("checklist_done") val checklistDone: Int = 0,
    @SerialName("checklist_in_progress") val checklistInProgress: Int = 0,
    @SerialName("next_checklist_item") val nextChecklistItem: ChecklistItem? = null,
    @SerialName("open_todos") val openTodos: List<Todo> = emptyList(),
    @SerialName("last_7_days") val last7Days: DayStat = DayStat(product = "", day = ""),
)

object Products {
    val names = linkedMapOf("mp3" to "Ampli MP3 Player", "carhub" to "Ampli Car Hub", "naukri" to "Ampli Naukri")
    private val shortNames = mapOf("mp3" to "MP3", "carhub" to "Car Hub", "naukri" to "Naukri")
    private val prices = mapOf("mp3" to "Pro ₹199 lifetime", "carhub" to "Pro ₹499 one-time", "naukri" to "Free · leads + ads")
    fun name(id: String?) = names[id] ?: "Ampli Tech"
    fun short(id: String?) = shortNames[id] ?: "All"
    fun price(id: String?) = prices[id] ?: ""
}

/** Indian digit grouping without a currency sign: 1,23,456. */
fun grouped(n: Long): String {
    val neg = n < 0
    val s = kotlin.math.abs(n).toString()
    val body = if (s.length <= 3) s else {
        val last3 = s.takeLast(3)
        val rest = s.dropLast(3).reversed().chunked(2).joinToString(",").reversed()
        "$rest,$last3"
    }
    return if (neg) "-$body" else body
}

/** Paise to rupees with Indian grouping: 1234500 -> ₹12,345. */
fun rupees(paise: Long): String {
    val r = paise / 100
    return if (r < 0) "-₹" + grouped(-r) else "₹" + grouped(r)
}

/** One decimal, without a trailing ".0": 1.0 -> "1", 1.25 -> "1.3". */
private fun oneDecimal(d: Double): String {
    val s = String.format(java.util.Locale.US, "%.1f", d)
    return if (s.endsWith(".0")) s.dropLast(2) else s
}

/** Short money for tight boxes: up to ₹99,999 in full, then ₹12.3L (lakh) and ₹4.5Cr (crore). */
fun compactRupees(paise: Long): String {
    val r = kotlin.math.abs(paise / 100)
    val sign = if (paise < 0) "-" else ""
    return when {
        r < 100_000L -> sign + "₹" + grouped(r)
        r < 10_000_000L -> sign + "₹" + oneDecimal(r / 100_000.0) + "L"
        else -> sign + "₹" + oneDecimal(r / 10_000_000.0) + "Cr"
    }
}

/** Short counts: up to 9,99,999 in full, then 12.5L and 1.2Cr. */
fun compactCount(n: Long): String {
    val a = kotlin.math.abs(n)
    val sign = if (n < 0) "-" else ""
    return when {
        a < 1_000_000L -> grouped(n)
        a < 10_000_000L -> sign + oneDecimal(a / 100_000.0) + "L"
        else -> sign + oneDecimal(a / 10_000_000.0) + "Cr"
    }
}

// ---------------- API v2: dashboard, ads manager, reviews (docs/api-v2.md) ----------------

@Serializable
data class Sentiment(
    val positive: Double = 0.0,
    val mixed: Double = 0.0,
    val negative: Double = 0.0,
)

@Serializable
data class WeekDownloads(val organic: Long = 0, val ads: Long = 0)

@Serializable
data class AdsSummary(
    @SerialName("spend_paise") val spendPaise: Long = 0,
    @SerialName("revenue_paise") val revenuePaise: Long = 0,
    val installs: Long = 0,
    val roas: Double = 0.0,
    @SerialName("cpi_paise") val cpiPaise: Long = 0,
    @SerialName("profit_paise") val profitPaise: Long = 0,
)

@Serializable
data class WidgetRow(
    val label: String = "",
    val value: Double = 0.0,
    val pct: Double = 0.0,
)

@Serializable
data class Widget(
    val id: String = "",
    val title: String = "",
    val kind: String = "bars", // funnel | bars | split
    val rows: List<WidgetRow> = emptyList(),
)

@Serializable
data class ProductSummary(
    val id: String = "",
    val name: String = "",
    @SerialName("business_paise") val businessPaise: Long = 0,
    val downloads: Long = 0,
    val rating: Double? = null,
    @SerialName("change_pct") val changePct: Double = 0.0,
)

@Serializable
data class Dashboard(
    val product: String = "all",
    @SerialName("business_paise") val businessPaise: Long = 0,
    @SerialName("business_change_pct") val businessChangePct: Double = 0.0,
    val downloads: Long = 0,
    @SerialName("downloads_week") val downloadsWeek: Long = 0,
    val rating: Double? = null,
    val reviews: Long = 0,
    @SerialName("active_users") val activeUsers: Long = 0,
    @SerialName("active_change_pct") val activeChangePct: Double = 0.0,
    @SerialName("pro_sales") val proSales: Long = 0,
    @SerialName("pro_revenue_paise") val proRevenuePaise: Long = 0,
    @SerialName("ad_revenue_paise") val adRevenuePaise: Long = 0,
    @SerialName("tips_paise") val tipsPaise: Long = 0,
    @SerialName("other_revenue_paise") val otherRevenuePaise: Long = 0,
    @SerialName("revenue_30d") val revenue30d: List<Long> = emptyList(),
    @SerialName("weekly_downloads") val weeklyDownloads: List<WeekDownloads> = emptyList(),
    val stars: List<Long> = emptyList(), // 5,4,3,2,1
    val sentiment: Sentiment = Sentiment(),
    val ads: AdsSummary = AdsSummary(),
    val widgets: List<Widget> = emptyList(),
    val products: List<ProductSummary> = emptyList(),
    /** Latest Play vitals for one product; null when unknown (and for "all"). Rates are fractions: 0.0042 = 0.42%. */
    val vitals: Vitals? = null,
)

@Serializable
data class Vitals(
    val day: String = "",
    @SerialName("crash_rate") val crashRate: Double? = null,
    @SerialName("anr_rate") val anrRate: Double? = null,
)

/** One "Needs your attention" item from GET /v1/alerts. */
@Serializable
data class Alert(
    val id: String = "",
    val severity: String = "low", // high | medium | low
    val product: String = "",
    val title: String = "",
    val detail: String = "",
    val action: String = "", // open_ads | open_reviews | open_inbox | open_settings | open_growth | ""
)

@Serializable
data class AdAccount(
    val id: Long = 0,
    val platform: String = "",
    @SerialName("external_id") val externalId: String = "",
    val name: String = "",
    val currency: String = "INR",
    val status: String = "connected",
    @SerialName("last_sync_at") val lastSyncAt: String? = null,
    @SerialName("last_error") val lastError: String = "",
)

@Serializable
data class AdsPlatform(
    val id: String = "",
    val name: String = "",
    val sub: String = "",
    val configured: Boolean = false,
    val accounts: List<AdAccount> = emptyList(),
    @SerialName("min_daily_budget_paise") val minDailyBudgetPaise: Long = 0,
)

@Serializable
data class Verdict(
    val code: String = "new", // scale | watch | stop | early | new
    val label: String = "",
    val reason: String = "",
)

@Serializable
data class AdCampaign(
    val id: Long = 0,
    val platform: String = "",
    val product: String = "",
    val name: String = "",
    val status: String = "draft", // draft | in_review | live | paused | ended | error
    val countries: List<String> = emptyList(),
    @SerialName("daily_budget_paise") val dailyBudgetPaise: Long = 0,
    @SerialName("spend_paise") val spendPaise: Long = 0,
    val impressions: Long = 0,
    val clicks: Long = 0,
    val installs: Long = 0,
    @SerialName("revenue_paise") val revenuePaise: Long = 0,
    val roas: Double = 0.0,
    @SerialName("cpi_paise") val cpiPaise: Long = 0,
    val days: Int = 0,
    val verdict: Verdict = Verdict(),
    @SerialName("ad_account_id") val adAccountId: Long? = null,
    @SerialName("utm_campaign") val utmCampaign: String = "",
    @SerialName("last_error") val lastError: String = "",
    @SerialName("auto_stop") val autoStop: AutoStop = AutoStop(),
    /** link = matched by our link, estimate = matched by platform and time, none = no installs matched yet. */
    val attribution: String = "none",
)

/** Pause a campaign automatically once ROAS < [minRoas] after [minDays] days and [minSpendPaise] spent. */
@Serializable
data class AutoStop(
    val enabled: Boolean = false,
    @SerialName("min_roas") val minRoas: Double = 0.5,
    @SerialName("min_days") val minDays: Int = 7,
    @SerialName("min_spend_paise") val minSpendPaise: Long = 200000,
    /** Set by the server when a sync found the campaign below the limit; the next sync 24 h later pauses it. */
    @SerialName("pending_since") val pendingSince: String? = null,
    /** After a manual resume the rule waits until this time. */
    @SerialName("hold_until") val holdUntil: String? = null,
)

@Serializable
data class CampaignLogEntry(
    val id: Long = 0,
    @SerialName("campaign_id") val campaignId: Long = 0,
    val actor: String = "", // owner | rule | sync
    val action: String = "",
    val detail: String = "",
    @SerialName("created_at") val createdAt: String = "",
)

/** POST /v1/ads/sync and /v1/reviews/sync: how many synced, plus any problems. */
@Serializable
data class SyncResult(
    val synced: Int = 0,
    val errors: List<String> = emptyList(),
)

@Serializable
data class AdmobStatus(
    val configured: Boolean = false,
    val connected: Boolean = false,
    val account: String = "",
    @SerialName("last_sync_at") val lastSyncAt: String? = null,
    @SerialName("last_error") val lastError: String = "",
)

@Serializable
data class PlayStatus(
    val configured: Boolean = false,
    @SerialName("last_sync_at") val lastSyncAt: String? = null,
    @SerialName("last_error") val lastError: String = "",
)

@Serializable
data class Integrations(
    val admob: AdmobStatus = AdmobStatus(),
    val play: PlayStatus = PlayStatus(),
)

@Serializable
data class IntegrationsSync(
    @SerialName("admob_days") val admobDays: Int = 0,
    val vitals: Int = 0,
    val reviews: Int = 0,
    @SerialName("play_days") val playDays: Int = 0,
    /** Refunds found by Play's voided-purchases report and taken off revenue. */
    val refunds: Int = 0,
    val errors: List<String> = emptyList(),
)

/** A Play product and its price; the server takes purchase prices from this table, not from the app. */
@Serializable
data class Sku(
    val product: String = "mp3",
    val sku: String = "",
    val kind: String = "pro", // pro | tip | subscription | other
    @SerialName("price_paise") val pricePaise: Long = 0,
)

@Serializable
data class AdsCampaignsResponse(
    val summary: AdsSummary = AdsSummary(),
    val campaigns: List<AdCampaign> = emptyList(),
)

@Serializable
data class Creative(
    val kind: String = "video", // video | image
    val url: String = "",
    @SerialName("thumbnail_url") val thumbnailUrl: String = "",
    val headline: String = "",
    val text: String = "",
)

@Serializable
data class NewCampaign(
    val product: String,
    @SerialName("ad_account_id") val adAccountId: Long,
    val name: String,
    val countries: List<String>,
    @SerialName("daily_budget_paise") val dailyBudgetPaise: Long,
    val days: Int,
    val creative: Creative,
    val launch: Boolean = true,
)

@Serializable
data class ReviewSummary(
    val total: Long = 0,
    val positive: Double = 0.0,
    val mixed: Double = 0.0,
    val negative: Double = 0.0,
    val stars: List<Long> = emptyList(),
)

@Serializable
data class ReviewTheme(val label: String = "", val count: Long = 0)

@Serializable
data class ReviewThemes(
    val praise: List<ReviewTheme> = emptyList(),
    val complaints: List<ReviewTheme> = emptyList(),
)

@Serializable
data class Review(
    val id: Long = 0,
    val product: String = "",
    val author: String = "",
    val rating: Int = 0,
    val body: String = "",
    val sentiment: String = "", // positive | mixed | negative
    val reply: String = "",
    val source: String = "play",
    @SerialName("created_at") val createdAt: String = "",
)

@Serializable
data class ReviewsResponse(
    val summary: ReviewSummary = ReviewSummary(),
    val themes: ReviewThemes = ReviewThemes(),
    val reviews: List<Review> = emptyList(),
)

// ---------------- growth: tracked links and cross-promo codes ----------------

/** A tracked Play link for one channel (GET /v1/links). Installs and money come from Play install referrers. */
@Serializable
data class TrackedLink(
    val id: Long = 0,
    val product: String = "mp3",
    val channel: String = "other",
    val label: String = "",
    val listing: String = "",
    @SerialName("from_product") val fromProduct: String = "",
    @SerialName("utm_campaign") val utmCampaign: String = "",
    @SerialName("created_at") val createdAt: String = "",
    val installs: Long = 0,
    @SerialName("installs_30d") val installs30d: Long = 0,
    @SerialName("pro_sales") val proSales: Long = 0,
    @SerialName("net_paise") val netPaise: Long = 0,
    @SerialName("channel_name") val channelName: String = "",
    val url: String = "",
)

@Serializable
data class NewLink(
    val product: String,
    val channel: String,
    val label: String = "",
    val listing: String = "",
    @SerialName("from_product") val fromProduct: String = "",
)

/** One cross-promo pair (GET /v1/promo): buyers of [sourceProduct] Pro get a code for [targetProduct]. */
@Serializable
data class PromoPair(
    @SerialName("source_product") val sourceProduct: String = "carhub",
    @SerialName("target_product") val targetProduct: String = "mp3",
    val sku: String = "ampli_pro",
    val imported: Long = 0,
    val available: Long = 0,
    val eligible: Long = 0,
    val claimed: Long = 0,
    val redeemed: Long = 0,
)

@Serializable
data class PromoImport(
    @SerialName("source_product") val sourceProduct: String,
    @SerialName("target_product") val targetProduct: String,
    val sku: String,
    val codes: List<String>,
)

@Serializable
data class PromoImportResult(val added: Int = 0, val skipped: Int = 0)

/** Where a tracked link is shared. Same ids and names as the server. */
object Channels {
    val names = linkedMapOf(
        "play_hindi" to "Hindi Play listing",
        "website" to "ampliapps.in",
        "whatsapp" to "WhatsApp",
        "telegram" to "Telegram",
        "instagram" to "Instagram (unpaid)",
        "youtube" to "YouTube",
        "crosspromo" to "More from Ampli",
        "other" to "Other link",
    )
    fun name(id: String) = names[id] ?: id

    /** Play package of each app, for the links built on the phone. */
    val packages = mapOf("mp3" to "com.vikash.ampli", "carhub" to "com.vikash.ampli.drive", "naukri" to "com.vikash.ampli.naukri")
}
