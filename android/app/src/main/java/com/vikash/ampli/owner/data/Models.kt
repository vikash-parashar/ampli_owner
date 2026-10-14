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
)

@Serializable
data class Campaign(
    val id: Long = 0,
    val platform: String,
    val product: String? = null,
    val name: String,
    val status: String = "draft",
    @SerialName("budget_paise") val budgetPaise: Long = 0,
    @SerialName("spend_paise") val spendPaise: Long = 0,
    val impressions: Long = 0,
    val clicks: Long = 0,
    val installs: Long = 0,
    @SerialName("post_url") val postUrl: String = "",
    @SerialName("starts_on") val startsOn: String? = null,
    @SerialName("ends_on") val endsOn: String? = null,
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
    val names = linkedMapOf("mp3" to "Ampli MP3 Player", "carhub" to "Ampli Car Hub")
    fun name(id: String?) = names[id] ?: "Ampli Tech"
}

fun rupees(paise: Long): String {
    val r = paise / 100
    // Indian grouping: 1,23,456
    val s = r.toString()
    if (s.length <= 3) return "₹$s"
    val last3 = s.takeLast(3)
    val rest = s.dropLast(3).reversed().chunked(2).joinToString(",").reversed()
    return "₹$rest,$last3"
}
