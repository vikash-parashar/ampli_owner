package com.vikash.ampli.owner.data

import java.time.Instant
import java.time.temporal.ChronoUnit
import java.util.Locale

/**
 * Deterministic sample numbers shown when no server is connected.
 * They match the approved preview (owner-mock), so the phone-only app looks the same as the mock.
 * Amounts below are in rupees and turned into paise on the way out.
 */
internal object Sample {

    /** Same seeded generator as the preview, so the revenue curves have the same shape. */
    private fun series(seed0: Long, n: Int, base: Double, growth: Double, noise: Double): List<Long> {
        var seed = seed0
        val out = ArrayList<Long>(n)
        for (i in 0 until n) {
            seed = (seed * 16807L) % 2147483647L
            val r = (seed - 1L).toDouble() / 2147483646.0
            val v = base * (1.0 + growth * i / n) * (1.0 + (r - 0.5) * noise)
            out.add(maxOf(0L, Math.round(v)))
        }
        return out
    }

    private class P(
        val id: String,
        val biz: Long,
        val change: Double,
        val downloads: Long,
        val week: Long,
        val rating: Double,
        val reviews: Long,
        val active: Long,
        val pro: Long,
        val proRev: Long,
        val adRev: Long,
        val other: Long,
        val tips: Long,
        val rev: List<Long>,
        val dl: List<WeekDownloads>,
        val stars: List<Long>,
        val sent: Sentiment,
    )

    private fun weeks(vararg pairs: Long): List<WeekDownloads> =
        pairs.toList().chunked(2).map { WeekDownloads(organic = it[0], ads = it[1]) }

    private val mp3 = P(
        id = "mp3", biz = 184300, change = 22.0, downloads = 12480, week = 1120, rating = 4.6, reviews = 842,
        active = 5310, pro = 612, proRev = 121788, adRev = 62512, other = 0, tips = 0,
        rev = series(7, 30, 4200.0, 1.1, 0.5),
        dl = weeks(310, 220, 360, 260, 410, 380, 450, 520),
        stars = listOf(612L, 131L, 46L, 21L, 32L), sent = Sentiment(81.0, 11.0, 8.0),
    )
    private val carhub = P(
        id = "carhub", biz = 96800, change = 31.0, downloads = 3920, week = 410, rating = 4.4, reviews = 268,
        active = 1870, pro = 194, proRev = 96800, adRev = 0, other = 0, tips = 0,
        rev = series(11, 30, 2100.0, 1.4, 0.7),
        dl = weeks(90, 60, 120, 90, 140, 150, 150, 210),
        stars = listOf(171L, 49L, 19L, 10L, 19L), sent = Sentiment(76.0, 13.0, 11.0),
    )
    private val naukri = P(
        id = "naukri", biz = 41250, change = 64.0, downloads = 21640, week = 3900, rating = 4.7, reviews = 1310,
        active = 14200, pro = 0, proRev = 0, adRev = 28750, other = 12500, tips = 0,
        rev = series(23, 30, 700.0, 2.4, 0.6),
        dl = weeks(900, 300, 1200, 500, 1500, 800, 2100, 1800),
        stars = listOf(1040L, 168L, 48L, 20L, 34L), sent = Sentiment(86.0, 9.0, 5.0),
    )
    private val apps = listOf(mp3, carhub, naukri)

    private val all: P by lazy {
        P(
            id = "all", biz = apps.sumOf { it.biz }, change = 29.0, downloads = apps.sumOf { it.downloads }, week = 5430,
            rating = 4.6, reviews = apps.sumOf { it.reviews }, active = apps.sumOf { it.active },
            pro = apps.sumOf { it.pro }, proRev = apps.sumOf { it.proRev }, adRev = apps.sumOf { it.adRev },
            other = apps.sumOf { it.other }, tips = apps.sumOf { it.tips },
            rev = (0 until 30).map { i -> apps.sumOf { it.rev[i] } },
            dl = (0 until 4).map { i -> WeekDownloads(apps.sumOf { it.dl[i].organic }, apps.sumOf { it.dl[i].ads }) },
            stars = (0 until 5).map { i -> apps.sumOf { it.stars[i] } },
            sent = Sentiment(83.0, 10.0, 7.0),
        )
    }

    private fun of(product: String): P = when (product) {
        "mp3" -> mp3
        "carhub" -> carhub
        "naukri" -> naukri
        else -> all
    }

    private fun row(label: String, value: Double, pct: Double) = WidgetRow(label, value, pct)

    private fun widgets(product: String): List<Widget> = when (product) {
        "mp3" -> listOf(
            Widget(
                "funnel", "Free to Pro funnel", "funnel",
                listOf(
                    row("Installed", 12480.0, 100.0),
                    row("Played 10+ songs", 8110.0, 65.0),
                    row("Opened a Pro feature", 2620.0, 21.0),
                    row("Tried Pro 24h (ad)", 1290.0, 10.3),
                    row("Bought Pro ₹199", 612.0, 4.9),
                ),
            ),
            Widget(
                "features", "Most used features", "bars",
                listOf(
                    row("Equalizer + bass boost", 71.0, 71.0),
                    row("Record player screen", 64.0, 64.0),
                    row("Sleep timer", 38.0, 38.0),
                    row("Ringtone maker", 22.0, 22.0),
                    row("Wi-Fi transfer", 9.0, 9.0),
                ),
            ),
            Widget(
                "money", "Money split", "split",
                listOf(row("Pro sales", 12178800.0, 66.0), row("AdMob", 6251200.0, 34.0)),
            ),
        )
        "carhub" -> listOf(
            Widget("voice", "Voice the drivers picked", "split", listOf(row("Bunty (male)", 58.0, 58.0), row("Simran (female)", 42.0, 42.0))),
            Widget(
                "commands", "Top voice commands", "bars",
                listOf(
                    row("Navigate to…", 44.0, 44.0),
                    row("Play music", 27.0, 27.0),
                    row("Call…", 14.0, 14.0),
                    row("FASTag balance", 9.0, 9.0),
                    row("SOS", 1.0, 1.0),
                ),
            ),
        )
        "naukri" -> listOf(
            Widget("alerts", "Alerts sent, 7 days", "bars", listOf(row("Job alerts", 182400.0, 100.0), row("Marked applied", 9310.0, 5.1))),
            Widget(
                "states", "Users by state", "bars",
                listOf(row("Haryana", 34.0, 34.0), row("Uttar Pradesh", 29.0, 29.0), row("Rajasthan", 21.0, 21.0), row("Delhi", 11.0, 11.0), row("Other", 5.0, 5.0)),
            ),
            Widget("money", "Money split", "split", listOf(row("AdMob", 2875000.0, 70.0), row("Coaching leads", 1250000.0, 30.0))),
        )
        else -> {
            val tot = all.biz.toDouble()
            listOf(
                Widget(
                    "money", "Where the money comes from", "split",
                    apps.map { p -> row(Products.short(p.id), p.biz * 100.0, p.biz * 100.0 / tot) },
                ),
            )
        }
    }

    fun dashboard(product: String, campaigns: List<AdCampaign>): Dashboard {
        val p = of(product)
        return Dashboard(
            product = product,
            businessPaise = p.biz * 100,
            businessChangePct = p.change,
            downloads = p.downloads,
            downloadsWeek = p.week,
            rating = p.rating,
            reviews = p.reviews,
            activeUsers = p.active,
            activeChangePct = 12.0,
            proSales = p.pro,
            proRevenuePaise = p.proRev * 100,
            adRevenuePaise = p.adRev * 100,
            tipsPaise = p.tips * 100,
            otherRevenuePaise = p.other * 100,
            revenue30d = p.rev.map { it * 100 },
            weeklyDownloads = p.dl,
            stars = p.stars,
            sentiment = p.sent,
            ads = summary(product, campaigns),
            widgets = widgets(product),
            products = if (product == "all") apps.map { a ->
                ProductSummary(a.id, Products.name(a.id), a.biz * 100, a.downloads, a.rating, a.change)
            } else emptyList(),
        )
    }

    // ---------------- ads ----------------

    fun platforms(): List<AdsPlatform> = listOf(
        AdsPlatform(
            "meta", "Meta", "Facebook + Instagram", true,
            listOf(AdAccount(1, "meta", "act_1001", "Ampli Studio", "INR", "connected", Instant.now().toString(), "")),
        ),
        AdsPlatform(
            "tiktok", "TikTok", "TikTok Ads", true,
            listOf(AdAccount(2, "tiktok", "7001", "Ampli Studio", "INR", "connected", Instant.now().toString(), "")),
        ),
        AdsPlatform("google", "Google Ads", "YouTube + Play + Search", true, emptyList()),
    )

    private fun f2(d: Double) = String.format(Locale.US, "%.2f", d)

    /** Same rules as the server: under 5 days = early; ROAS 1.5+ scale; 0.8-1.5 watch; under 0.8 stop. */
    fun verdict(status: String, days: Int, roas: Double): Verdict = when {
        status == "in_review" || status == "draft" -> Verdict("new", "In review", "Waiting for platform approval.")
        days < 5 -> Verdict("early", "Too early", "Only $days days of data. Give it 5 days before deciding.")
        roas >= 1.5 -> Verdict("scale", "Scale up", "Every ₹1 spent brought back ₹${f2(roas)}. Raise the budget 20%.")
        roas >= 0.8 -> Verdict("watch", "Watch", "ROAS ${f2(roas)}x is near break-even. Try a new creative before adding money.")
        else -> Verdict("stop", "Stop", "Only ₹${f2(roas)} back per ₹1. Pause this and move the budget to a winner.")
    }

    private fun camp(
        id: Long, product: String, platform: String, name: String, country: String,
        spend: Long, installs: Long, revenue: Long, days: Int,
    ): AdCampaign {
        val roas = if (spend == 0L) 0.0 else revenue.toDouble() / spend
        return AdCampaign(
            id = id, platform = platform, product = product, name = name, status = "live",
            countries = listOf(country),
            dailyBudgetPaise = (spend * 100 / days) / 10000 * 10000,
            spendPaise = spend * 100, impressions = installs * 40, clicks = installs * 3, installs = installs,
            revenuePaise = revenue * 100, roas = roas,
            cpiPaise = if (installs == 0L) 0L else spend * 100 / installs,
            days = days, verdict = verdict("live", days, roas),
            adAccountId = when (platform) { "meta" -> 1L; "tiktok" -> 2L; else -> null },
            utmCampaign = "ampli-c$id",
        )
    }

    fun campaigns(): List<AdCampaign> = listOf(
        camp(1, "mp3", "meta", "MP3 · Reels · Pay once, play forever", "IN", 18000, 3420, 41200, 12),
        camp(2, "mp3", "tiktok", "MP3 · TikTok · Record player", "US", 24500, 610, 9800, 9),
        camp(3, "carhub", "google", "Car Hub · YouTube · Haryanvi voice", "IN", 12000, 1480, 33900, 14),
        camp(4, "carhub", "meta", "Car Hub · Instagram · Hey Ampli", "IN", 8000, 520, 7480, 6),
        camp(5, "naukri", "google", "Naukri · App campaign · Sarkari jobs", "IN", 15000, 9400, 21300, 10),
        camp(6, "naukri", "meta", "Naukri · Facebook · Haryana police", "IN", 6000, 1900, 2100, 4),
    )

    fun summary(product: String, campaigns: List<AdCampaign>): AdsSummary {
        val cs = campaigns.filter { (product == "all" || it.product == product) && it.status != "in_review" && it.status != "draft" }
        val spend = cs.sumOf { it.spendPaise }
        val rev = cs.sumOf { it.revenuePaise }
        val inst = cs.sumOf { it.installs }
        return AdsSummary(
            spendPaise = spend, revenuePaise = rev, installs = inst,
            roas = if (spend == 0L) 0.0 else rev.toDouble() / spend,
            cpiPaise = if (inst == 0L) 0L else spend / inst,
            profitPaise = rev - spend,
        )
    }

    // ---------------- reviews ----------------

    private fun ago(hours: Long): String = Instant.now().minus(hours, ChronoUnit.HOURS).truncatedTo(ChronoUnit.SECONDS).toString()

    fun reviews(): List<Review> = listOf(
        Review(1, "mp3", "Rohit S.", 5, "Record player screen is beautiful and no monthly fee. Paid ₹199 happily.", "positive", "", "play", ago(2)),
        Review(2, "naukri", "Pooja", 5, "Haryana police bharti ka alert sabse pehle isi app pe aaya. Thank you!", "positive", "", "play", ago(3)),
        Review(3, "carhub", "Sandeep Dahiya", 5, "Bunty ki awaaz sun ke hasi aa jaati hai. Asli Haryanvi lagta hai 😂", "positive", "", "play", ago(5)),
        Review(4, "mp3", "Ankit", 2, "Ads after every few songs are annoying. Make Pro cheaper?", "negative", "", "play", ago(8)),
        Review(5, "carhub", "Manoj", 3, "Voice is great but 'Hey Ampli' misses sometimes on the highway.", "mixed", "", "play", ago(26)),
        Review(6, "naukri", "Kiran", 1, "Login OTP did not come.", "negative", "", "play", ago(30)),
        Review(7, "mp3", "Meera", 4, "Love the EQ. Please add lyrics.", "positive", "", "play", ago(50)),
    )

    fun reviewSummary(product: String): ReviewSummary {
        val p = of(product)
        return ReviewSummary(p.reviews, p.sent.positive, p.sent.mixed, p.sent.negative, p.stars)
    }

    val themes = ReviewThemes(
        praise = listOf(ReviewTheme("Record player", 212), ReviewTheme("No subscription", 180), ReviewTheme("Haryanvi voice", 141), ReviewTheme("Fast alerts", 133)),
        complaints = listOf(ReviewTheme("Too many ads", 64), ReviewTheme("Voice misses", 22), ReviewTheme("OTP delay", 18), ReviewTheme("Wants lyrics", 15)),
    )
}
