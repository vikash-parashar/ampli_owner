package com.vikash.ampli.owner.data

import android.content.Context
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.KSerializer
import kotlinx.serialization.Serializable
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import java.io.File
import java.net.HttpURLConnection
import java.net.URL
import java.time.Instant
import java.time.LocalDate

val json = Json { ignoreUnknownKeys = true; explicitNulls = false; encodeDefaults = true }

/** Everything the screens need. [LocalRepo] works offline on this phone; [RemoteRepo] talks to the Go server. */
interface Repo {
    val isRemote: Boolean
    suspend fun overview(): Overview
    suspend fun tickets(kind: String? = null): List<Ticket>
    suspend fun ticket(id: Long): TicketDetail
    suspend fun createTicket(t: Ticket): Ticket
    suspend fun setTicketStatus(id: Long, status: String)
    suspend fun reply(id: Long, body: String)
    suspend fun checklist(): List<ChecklistItem>
    suspend fun setChecklistStatus(id: String, status: String)
    suspend fun todos(): List<Todo>
    suspend fun addTodo(title: String, due: String?, product: String?)
    suspend fun setTodoDone(id: Long, done: Boolean)
    suspend fun deleteTodo(id: Long)
    suspend fun features(): List<Feature>
    suspend fun addFeature(f: Feature)
    suspend fun setFeatureState(id: Long, state: String)
    suspend fun stats(from: String, to: String): List<DayStat>
    suspend fun putStat(d: DayStat)
    suspend fun campaigns(): List<Campaign>
    suspend fun saveCampaign(c: Campaign)
    suspend fun guides(): List<Guide>

    // API v2 (docs/api-v2.md). Product is "all", "mp3", "carhub" or "naukri".
    suspend fun dashboard(product: String): Dashboard
    suspend fun adPlatforms(): List<AdsPlatform>
    /** Starts the OAuth sign-in for an ad platform and returns the URL to open in the browser. */
    suspend fun connectAds(platform: String): String
    suspend fun deleteAdAccount(id: Long)
    suspend fun adCampaigns(product: String): AdsCampaignsResponse
    suspend fun createCampaign(c: NewCampaign): AdCampaign
    suspend fun pauseCampaign(id: Long): AdCampaign
    suspend fun resumeCampaign(id: Long): AdCampaign
    suspend fun setBudget(id: Long, dailyBudgetPaise: Long): AdCampaign
    /** Pulls spend, clicks and installs from every connected ad account; returns how many synced. */
    suspend fun syncAds(): Int
    suspend fun reviews(product: String): ReviewsResponse
    suspend fun replyReview(id: Long, text: String)
}

// ---------------- local (on-device JSON file, seeded from assets) ----------------

@Serializable
private data class LocalDb(
    val tickets: List<Ticket> = emptyList(),
    val messages: List<Message> = emptyList(),
    val checklist: List<ChecklistItem> = emptyList(),
    val todos: List<Todo> = emptyList(),
    val features: List<Feature> = emptyList(),
    val stats: List<DayStat> = emptyList(),
    val campaigns: List<Campaign> = emptyList(),
    val guides: List<Guide> = emptyList(),
    val nextId: Long = 1000,
)

class LocalRepo(private val context: Context) : Repo {
    override val isRemote = false
    private val file = File(context.filesDir, "owner-db.json")
    private val lock = Mutex()
    private var cache: LocalDb? = null

    private fun <T> asset(name: String, s: KSerializer<T>): T =
        json.decodeFromString(s, context.assets.open(name).bufferedReader().use { it.readText() })

    private fun load(): LocalDb {
        cache?.let { return it }
        val db = if (file.exists()) json.decodeFromString(LocalDb.serializer(), file.readText())
        else LocalDb(
            checklist = asset("checklist.json", ListSerializer(ChecklistItem.serializer())),
            features = asset("features.json", ListSerializer(Feature.serializer())),
            guides = asset("guides.json", ListSerializer(Guide.serializer())),
        )
        cache = db
        return db
    }

    private suspend fun <T> read(f: (LocalDb) -> T): T = withContext(Dispatchers.IO) { lock.withLock { f(load()) } }

    private suspend fun write(f: (LocalDb) -> LocalDb) = withContext(Dispatchers.IO) {
        lock.withLock {
            val next = f(load())
            val tmp = File(file.path + ".tmp")
            tmp.writeText(json.encodeToString(LocalDb.serializer(), next))
            tmp.renameTo(file)
            cache = next
        }
    }

    private fun now() = Instant.now().toString()

    override suspend fun overview() = read { db ->
        val open = db.tickets.filter { it.status !in setOf("fixed", "closed") }
        val since = LocalDate.now().minusDays(6).toString()
        val week = db.stats.filter { it.day >= since }
        Overview(
            openTickets = open.size,
            waitingOnOwner = open.count { it.status in setOf("new", "waiting_on_owner") },
            ticketsByKind = open.groupingBy { it.kind }.eachCount(),
            checklistTotal = db.checklist.size,
            checklistDone = db.checklist.count { it.status == "done" },
            checklistInProgress = db.checklist.count { it.status == "in-progress" },
            nextChecklistItem = db.checklist.filter { it.status != "done" }
                .sortedWith(compareBy<ChecklistItem>({ it.status != "in-progress" }, { it.order })).firstOrNull(),
            openTodos = db.todos.filter { !it.done }.sortedWith(compareBy(nullsLast<String>()) { it.due }).take(5),
            last7Days = DayStat(
                product = "", day = "",
                installs = week.sumOf { it.installs }, uninstalls = week.sumOf { it.uninstalls },
                reviews = week.sumOf { it.reviews }, proSales = week.sumOf { it.proSales },
                proRevenuePaise = week.sumOf { it.proRevenuePaise }, adRevenuePaise = week.sumOf { it.adRevenuePaise },
            ),
        )
    }

    override suspend fun tickets(kind: String?) = read { db ->
        db.tickets.filter { kind == null || it.kind == kind }.sortedByDescending { it.id }
    }

    override suspend fun ticket(id: Long) = read { db ->
        TicketDetail(db.tickets.first { it.id == id }, db.messages.filter { it.ticketId == id })
    }

    override suspend fun createTicket(t: Ticket): Ticket {
        var made = t
        write { db ->
            made = t.copy(id = db.nextId, ref = "T-%04d".format(db.nextId), createdAt = now(), updatedAt = now())
            db.copy(tickets = db.tickets + made, nextId = db.nextId + 1)
        }
        return made
    }

    override suspend fun setTicketStatus(id: Long, status: String) = write { db ->
        db.copy(tickets = db.tickets.map { if (it.id == id) it.copy(status = status, updatedAt = now()) else it })
    }

    override suspend fun reply(id: Long, body: String) = write { db ->
        val m = Message(id = db.nextId, ticketId = id, author = "owner", body = body, createdAt = now())
        db.copy(
            messages = db.messages + m, nextId = db.nextId + 1,
            tickets = db.tickets.map {
                if (it.id == id && it.status in setOf("new", "waiting_on_owner")) it.copy(status = "waiting_on_user") else it
            },
        )
    }

    override suspend fun checklist() = read { it.checklist.sortedBy { c -> c.order } }

    override suspend fun setChecklistStatus(id: String, status: String) = write { db ->
        db.copy(checklist = db.checklist.map {
            if (it.id == id) it.copy(status = status, doneAt = if (status == "done") it.doneAt ?: now() else null) else it
        })
    }

    override suspend fun todos() = read { db -> db.todos.sortedWith(compareBy<Todo>({ it.done }, { it.due ?: "9999" })) }

    override suspend fun addTodo(title: String, due: String?, product: String?) = write { db ->
        db.copy(todos = db.todos + Todo(db.nextId, title, due, product, false, now()), nextId = db.nextId + 1)
    }

    override suspend fun setTodoDone(id: Long, done: Boolean) = write { db ->
        db.copy(todos = db.todos.map { if (it.id == id) it.copy(done = done) else it })
    }

    override suspend fun deleteTodo(id: Long) = write { db -> db.copy(todos = db.todos.filter { it.id != id }) }

    override suspend fun features() = read { it.features.sortedWith(compareBy<Feature>({ f -> f.product }, { f -> f.order })) }

    override suspend fun addFeature(f: Feature) = write { db ->
        val order = (db.features.filter { it.product == f.product }.maxOfOrNull { it.order } ?: 0) + 10
        db.copy(features = db.features + f.copy(id = db.nextId, order = order), nextId = db.nextId + 1)
    }

    override suspend fun setFeatureState(id: Long, state: String) = write { db ->
        db.copy(features = db.features.map { if (it.id == id) it.copy(state = state) else it })
    }

    override suspend fun stats(from: String, to: String) = read { db ->
        db.stats.filter { it.day in from..to }.sortedBy { it.day }
    }

    override suspend fun putStat(d: DayStat) = write { db ->
        db.copy(stats = db.stats.filterNot { it.product == d.product && it.day == d.day } + d)
    }

    override suspend fun campaigns() = read { it.campaigns.sortedByDescending { c -> c.id } }

    override suspend fun saveCampaign(c: Campaign) = write { db ->
        if (c.id == 0L) db.copy(campaigns = db.campaigns + c.copy(id = db.nextId), nextId = db.nextId + 1)
        else db.copy(campaigns = db.campaigns.map { if (it.id == c.id) c else it })
    }

    override suspend fun guides() = read { it.guides }

    // ---- API v2 in phone-only mode: deterministic sample data, changes kept in memory only ----

    private val mem = Mutex()
    private var sampleCampaigns: List<AdCampaign> = Sample.campaigns()
    private var samplePlatforms: List<AdsPlatform> = Sample.platforms()
    private var sampleReviews: List<Review> = Sample.reviews()

    private suspend fun <T> inMemory(f: () -> T): T = mem.withLock { f() }

    override suspend fun dashboard(product: String): Dashboard = inMemory { Sample.dashboard(product, sampleCampaigns) }

    override suspend fun adPlatforms(): List<AdsPlatform> = inMemory { samplePlatforms }

    override suspend fun connectAds(platform: String): String =
        throw IllegalStateException("Connect a server in Settings first")

    override suspend fun deleteAdAccount(id: Long) = inMemory {
        samplePlatforms = samplePlatforms.map { p -> p.copy(accounts = p.accounts.filter { it.id != id }) }
    }

    override suspend fun adCampaigns(product: String): AdsCampaignsResponse = inMemory {
        AdsCampaignsResponse(
            summary = Sample.summary(product, sampleCampaigns),
            campaigns = sampleCampaigns.filter { product == "all" || it.product == product },
        )
    }

    override suspend fun createCampaign(c: NewCampaign): AdCampaign = inMemory {
        val id = (sampleCampaigns.maxOfOrNull { it.id } ?: 0L) + 1L
        val platform = samplePlatforms.firstOrNull { p -> p.accounts.any { it.id == c.adAccountId } }?.id ?: "meta"
        val status = if (c.launch) "in_review" else "paused"
        val made = AdCampaign(
            id = id, platform = platform, product = c.product, name = c.name, status = status,
            countries = c.countries, dailyBudgetPaise = c.dailyBudgetPaise, days = 0,
            verdict = Sample.verdict("in_review", 0, 0.0), adAccountId = c.adAccountId, utmCampaign = "ampli-c$id",
        )
        sampleCampaigns = listOf(made) + sampleCampaigns
        made
    }

    private fun changeCampaign(id: Long, f: (AdCampaign) -> AdCampaign): AdCampaign {
        val old = sampleCampaigns.firstOrNull { it.id == id } ?: throw IllegalArgumentException("No campaign $id")
        val changed = f(old)
        sampleCampaigns = sampleCampaigns.map { if (it.id == id) changed else it }
        return changed
    }

    override suspend fun pauseCampaign(id: Long): AdCampaign = inMemory { changeCampaign(id) { it.copy(status = "paused") } }

    override suspend fun resumeCampaign(id: Long): AdCampaign = inMemory {
        changeCampaign(id) { c ->
            val status = if (c.status == "draft") "in_review" else if (c.days == 0) "in_review" else "live"
            c.copy(status = status, verdict = Sample.verdict(status, c.days, c.roas))
        }
    }

    override suspend fun setBudget(id: Long, dailyBudgetPaise: Long): AdCampaign =
        inMemory { changeCampaign(id) { it.copy(dailyBudgetPaise = dailyBudgetPaise) } }

    override suspend fun syncAds(): Int = 0

    override suspend fun reviews(product: String): ReviewsResponse = inMemory {
        ReviewsResponse(
            summary = Sample.reviewSummary(product),
            themes = Sample.themes,
            reviews = sampleReviews.filter { product == "all" || it.product == product },
        )
    }

    override suspend fun replyReview(id: Long, text: String) = inMemory {
        sampleReviews = sampleReviews.map { if (it.id == id) it.copy(reply = text) else it }
    }
}

// ---------------- remote (Go server) ----------------

class HttpError(val code: Int, message: String) : Exception(message)

class RemoteRepo(baseUrl: String, private val token: String) : Repo {
    override val isRemote = true
    private val base = baseUrl.trimEnd('/')

    private suspend fun call(method: String, path: String, body: String? = null): String = withContext(Dispatchers.IO) {
        val conn = URL(base + path).openConnection() as HttpURLConnection
        try {
            conn.requestMethod = if (method == "PATCH") "POST" else method
            if (method == "PATCH") conn.setRequestProperty("X-HTTP-Method-Override", "PATCH")
            conn.connectTimeout = 10_000
            conn.readTimeout = 20_000
            conn.setRequestProperty("Authorization", "Bearer $token")
            conn.setRequestProperty("Accept", "application/json")
            if (body != null) {
                conn.doOutput = true
                conn.setRequestProperty("Content-Type", "application/json")
                conn.outputStream.use { it.write(body.toByteArray()) }
            }
            val code = conn.responseCode
            val text = (if (code < 400) conn.inputStream else conn.errorStream)?.bufferedReader()?.use { it.readText() } ?: ""
            if (code >= 400) {
                val msg = runCatching { json.decodeFromString(JsonObject.serializer(), text)["error"] }
                    .getOrNull()?.let { (it as? JsonPrimitive)?.content } ?: "HTTP $code"
                throw HttpError(code, msg)
            }
            text
        } finally {
            conn.disconnect()
        }
    }

    private suspend fun <T> get(path: String, s: KSerializer<T>): T = json.decodeFromString(s, call("GET", path))

    override suspend fun overview() = get("/v1/overview", Overview.serializer())
    override suspend fun tickets(kind: String?) =
        get("/v1/tickets" + (kind?.let { "?kind=$it" } ?: ""), ListSerializer(Ticket.serializer()))
    override suspend fun ticket(id: Long) = get("/v1/tickets/$id", TicketDetail.serializer())
    override suspend fun createTicket(t: Ticket) =
        json.decodeFromString(Ticket.serializer(), call("POST", "/v1/tickets", json.encodeToString(Ticket.serializer(), t)))

    override suspend fun setTicketStatus(id: Long, status: String) {
        call("PATCH", "/v1/tickets/$id", buildJsonObject { put("status", status) }.toString())
    }

    override suspend fun reply(id: Long, body: String) {
        call("POST", "/v1/tickets/$id/messages", buildJsonObject { put("body", body) }.toString())
    }

    override suspend fun checklist() = get("/v1/checklist", ListSerializer(ChecklistItem.serializer()))
    override suspend fun setChecklistStatus(id: String, status: String) {
        call("PATCH", "/v1/checklist/$id", buildJsonObject { put("status", status) }.toString())
    }

    override suspend fun todos() = get("/v1/todos", ListSerializer(Todo.serializer()))
    override suspend fun addTodo(title: String, due: String?, product: String?) {
        call("POST", "/v1/todos", buildJsonObject {
            put("title", title); due?.let { put("due", it) }; product?.let { put("product", it) }
        }.toString())
    }

    override suspend fun setTodoDone(id: Long, done: Boolean) {
        call("PATCH", "/v1/todos/$id", buildJsonObject { put("done", done) }.toString())
    }

    override suspend fun deleteTodo(id: Long) {
        call("DELETE", "/v1/todos/$id")
    }

    override suspend fun features() = get("/v1/features", ListSerializer(Feature.serializer()))
    override suspend fun addFeature(f: Feature) {
        call("POST", "/v1/features", buildJsonObject {
            put("product", f.product); put("title", f.title); put("state", f.state)
        }.toString())
    }

    override suspend fun setFeatureState(id: Long, state: String) {
        call("PATCH", "/v1/features/$id", buildJsonObject { put("state", state) }.toString())
    }

    override suspend fun stats(from: String, to: String) = get("/v1/stats?from=$from&to=$to", ListSerializer(DayStat.serializer()))
    override suspend fun putStat(d: DayStat) {
        call("PUT", "/v1/stats", json.encodeToString(ListSerializer(DayStat.serializer()), listOf(d)))
    }

    override suspend fun campaigns() = get("/v1/campaigns", ListSerializer(Campaign.serializer()))
    override suspend fun saveCampaign(c: Campaign) {
        val body = json.encodeToString(Campaign.serializer(), c)
        if (c.id == 0L) call("POST", "/v1/campaigns", body) else call("PUT", "/v1/campaigns/${c.id}", body)
    }

    override suspend fun guides() = get("/v1/guides", ListSerializer(Guide.serializer()))

    // ---- API v2 ----

    private suspend fun postCampaign(path: String, body: String = "{}"): AdCampaign =
        json.decodeFromString(AdCampaign.serializer(), call("POST", path, body))

    override suspend fun dashboard(product: String): Dashboard = get("/v1/dashboard?product=$product", Dashboard.serializer())

    override suspend fun adPlatforms(): List<AdsPlatform> = get("/v1/ads/platforms", ListSerializer(AdsPlatform.serializer()))

    override suspend fun connectAds(platform: String): String {
        val o = json.decodeFromString(JsonObject.serializer(), call("POST", "/v1/ads/connect/$platform", "{}"))
        val url = (o["auth_url"] as? JsonPrimitive)?.content ?: ""
        if (url.isBlank() || url == "null") throw IllegalStateException("The server did not send a sign-in link")
        return url
    }

    override suspend fun deleteAdAccount(id: Long) {
        call("DELETE", "/v1/ads/accounts/$id")
    }

    override suspend fun adCampaigns(product: String): AdsCampaignsResponse =
        get("/v1/ads/campaigns?product=$product", AdsCampaignsResponse.serializer())

    override suspend fun createCampaign(c: NewCampaign): AdCampaign =
        postCampaign("/v1/ads/campaigns", json.encodeToString(NewCampaign.serializer(), c))

    override suspend fun pauseCampaign(id: Long): AdCampaign = postCampaign("/v1/ads/campaigns/$id/pause")

    override suspend fun resumeCampaign(id: Long): AdCampaign = postCampaign("/v1/ads/campaigns/$id/resume")

    override suspend fun setBudget(id: Long, dailyBudgetPaise: Long): AdCampaign =
        postCampaign("/v1/ads/campaigns/$id/budget", buildJsonObject { put("daily_budget_paise", dailyBudgetPaise) }.toString())

    override suspend fun syncAds(): Int {
        val o = json.decodeFromString(JsonObject.serializer(), call("POST", "/v1/ads/sync", "{}"))
        return (o["synced"] as? JsonPrimitive)?.content?.toIntOrNull() ?: 0
    }

    override suspend fun reviews(product: String): ReviewsResponse = get("/v1/reviews?product=$product", ReviewsResponse.serializer())

    override suspend fun replyReview(id: Long, text: String) {
        call("POST", "/v1/reviews/$id/reply", buildJsonObject { put("text", text) }.toString())
    }
}

/** Server address and owner token, kept in private app prefs on this phone. */
class Settings(context: Context) {
    private val prefs = context.getSharedPreferences("owner", Context.MODE_PRIVATE)
    var serverUrl: String
        get() = prefs.getString("server_url", "") ?: ""
        set(v) = prefs.edit().putString("server_url", v.trim()).apply()
    var token: String
        get() = prefs.getString("token", "") ?: ""
        set(v) = prefs.edit().putString("token", v.trim()).apply()
}
