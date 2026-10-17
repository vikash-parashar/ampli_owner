package com.vikash.ampli.owner.ui

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.background
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.vikash.ampli.owner.data.Products
import com.vikash.ampli.owner.data.Review
import com.vikash.ampli.owner.data.ReviewTheme
import com.vikash.ampli.owner.data.ReviewsResponse
import com.vikash.ampli.owner.data.grouped
import java.time.Duration
import java.time.Instant
import java.time.OffsetDateTime

/** "2 h ago" from an ISO-8601 time; falls back to the date. */
fun ago(iso: String): String = runCatching {
    val mins = Duration.between(OffsetDateTime.parse(iso).toInstant(), Instant.now()).toMinutes()
    when {
        mins < 60 -> "${mins.coerceAtLeast(1L)} min ago"
        mins < 60 * 24 -> "${mins / 60} h ago"
        else -> "${mins / (60 * 24)} d ago"
    }
}.getOrDefault(iso.take(10))

@Composable
fun ReviewsScreen(nav: Nav, modifier: Modifier) {
    val product = nav.product
    var replying by remember { mutableStateOf<Review?>(null) }
    var adding by remember { mutableStateOf(false) }
    Loadable(
        key = nav.tick to product,
        load = { Pair(product, nav.repo.reviews(product)) },
        onRetry = nav::refresh,
        keepStale = true,
    ) { loaded ->
        ReviewsContent(
            key = loaded.first,
            data = loaded.second,
            modifier = modifier,
            remote = nav.repo.isRemote,
            onSync = {
                nav.run {
                    val r = nav.repo.syncReviews()
                    nav.toast(r.errors.firstOrNull() ?: "${r.synced} new from Google Play")
                }
            },
            onAdd = { adding = true },
            onReply = { replying = it },
        )
    }
    replying?.let { r ->
        ReplyDialog(nav, r, onDismiss = { replying = null })
    }
    if (adding) {
        AddReviewDialog(nav, if (product == "all") "mp3" else product, onDismiss = { adding = false })
    }
}

@Composable
private fun ReviewsContent(
    key: String,
    data: ReviewsResponse,
    modifier: Modifier,
    remote: Boolean,
    onSync: () -> Unit,
    onAdd: () -> Unit,
    onReply: (Review) -> Unit,
) {
    val a = ampli()
    val s = data.summary
    ScrollPage(modifier, key) {
        var n = 0

        // ---- actions: pull from Play, or type one that came by email or WhatsApp ----
        Row(
            Modifier.fillMaxWidth().rise(key, n++),
            horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End),
        ) {
            if (remote) SmallButton("Sync from Play", BtnKind.Ghost) { onSync() }
            SmallButton("Add review", BtnKind.Primary) { onAdd() }
        }

        AmpliCard(Modifier.rise(key, n++)) {
            CardTitle("How people feel", "${grouped(s.total)} reviews")
            if (s.total == 0L) {
                Text(
                    "No reviews yet", fontSize = 13.sp, color = a.muted, textAlign = TextAlign.Center,
                    modifier = Modifier.fillMaxWidth().padding(vertical = 10.dp),
                )
            } else {
                SentimentGauge(
                    listOf(s.positive.toFloat() to a.good, s.mixed.toFloat() to a.warn, s.negative.toFloat() to a.bad),
                    key,
                )
                // Wraps onto two lines on narrow screens instead of running off the card.
                Wrap(Modifier.fillMaxWidth().padding(top = 8.dp), spacing = 14.dp, center = true) {
                    Text("😊 ${Math.round(s.positive)}% positive", fontSize = 12.sp, fontWeight = FontWeight.Bold, color = a.good, maxLines = 1)
                    Text("${Math.round(s.mixed)}% mixed", fontSize = 12.sp, fontWeight = FontWeight.Bold, color = a.warn, maxLines = 1)
                    Text("${Math.round(s.negative)}% negative 😞", fontSize = 12.sp, fontWeight = FontWeight.Bold, color = a.bad, maxLines = 1)
                }
            }
        }

        AmpliCard(Modifier.rise(key, n++)) {
            CardTitle("What they praise")
            ThemeTags(data.themes.praise, good = true)
            Spacer(Modifier.height(14.dp))
            CardTitle("What they complain about")
            ThemeTags(data.themes.complaints, good = false)
        }

        AmpliCard(Modifier.rise(key, n++)) {
            CardTitle("Latest reviews", "reply from here")
            if (data.reviews.isEmpty()) {
                Text(
                    "No reviews yet.", fontSize = 13.sp, color = a.muted, textAlign = TextAlign.Center,
                    modifier = Modifier.fillMaxWidth().padding(10.dp),
                )
            }
            data.reviews.forEachIndexed { i, r ->
                if (i > 0) HorizontalDivider(color = a.line)
                ReviewItem(r) { onReply(r) }
            }
        }
    }
}

@Composable
private fun ThemeTags(themes: List<ReviewTheme>, good: Boolean) {
    val a = ampli()
    if (themes.isEmpty()) {
        Text("Nothing yet", fontSize = 12.sp, color = a.muted)
    } else Wrap(spacing = 6.dp) {
        themes.forEach { t ->
            Text(
                "${t.label} · ${grouped(t.count)}",
                fontSize = 11.5.sp,
                fontWeight = FontWeight.Bold,
                color = if (good) a.good else a.bad,
                maxLines = 1,
                modifier = Modifier
                    .padding(bottom = 6.dp)
                    .clip(RoundedCornerShape(50))
                    .background(if (good) a.goodBg else a.badBg)
                    .padding(horizontal = 10.dp, vertical = 6.dp),
            )
        }
    }
}

@Composable
private fun ReviewItem(r: Review, onReply: () -> Unit) {
    val a = ampli()
    Column(Modifier.fillMaxWidth().padding(vertical = 12.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                "${r.author.ifBlank { "Someone" }} · ${Products.short(r.product)}",
                fontSize = 12.sp, fontWeight = FontWeight.Bold, maxLines = 1, overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f),
            )
            val stars = r.rating.coerceIn(0, 5)
            Text(
                buildAnnotatedString {
                    withStyle(SpanStyle(color = Star)) { append("★".repeat(stars)) }
                    withStyle(SpanStyle(color = a.faint)) { append("★".repeat(5 - stars)) }
                },
                fontSize = 12.sp, letterSpacing = 1.sp,
            )
        }
        Text(r.body, fontSize = 12.5.sp, lineHeight = 19.sp, modifier = Modifier.padding(top = 6.dp))
        if (r.reply.isNotBlank()) {
            Text(
                "You replied: ${r.reply}", fontSize = 12.sp, color = accent(), lineHeight = 17.sp,
                modifier = Modifier.padding(top = 6.dp),
            )
        }
        Row(Modifier.fillMaxWidth().padding(top = 8.dp), verticalAlignment = Alignment.CenterVertically) {
            val where = if (r.source == "play") "Google Play" else r.source.replaceFirstChar { it.uppercase() }
            Text("${ago(r.createdAt)} · $where", fontSize = 11.sp, color = a.muted, modifier = Modifier.weight(1f))
            SmallButton(if (r.reply.isBlank()) "Reply" else "Edit reply", BtnKind.Ghost) { onReply() }
        }
    }
}

/** Play allows replies up to 350 characters. */
private const val REPLY_MAX = 350

@Composable
private fun ReplyDialog(nav: Nav, r: Review, onDismiss: () -> Unit) {
    var text by remember { mutableStateOf(r.reply.take(REPLY_MAX)) }
    var sending by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    val toPlay = r.source == "play"
    AlertDialog(
        onDismissRequest = { if (!sending) onDismiss() },
        title = { Text("Reply to ${r.author.ifBlank { "this review" }}") },
        text = {
            Column {
                Text(r.body, fontSize = 12.5.sp, color = ampli().muted, maxLines = 4, overflow = TextOverflow.Ellipsis)
                Spacer(Modifier.height(10.dp))
                OutlinedTextField(
                    value = text,
                    onValueChange = { text = it.take(REPLY_MAX); error = null },
                    label = { Text(if (toPlay) "Your reply (posts to Google Play)" else "Your reply") },
                    supportingText = {
                        Text("${text.length} / $REPLY_MAX", textAlign = TextAlign.End, modifier = Modifier.fillMaxWidth())
                    },
                    minLines = 3,
                    enabled = !sending,
                    keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Sentences),
                    modifier = Modifier.fillMaxWidth(),
                )
                error?.let { Text(it, color = ampli().bad, fontSize = 12.sp) }
            }
        },
        confirmButton = {
            TextButton(enabled = text.isNotBlank() && !sending, onClick = {
                sending = true
                error = null
                nav.send(
                    done = if (toPlay) "Reply posted to Google Play" else "Reply saved",
                    onOk = { sending = false; onDismiss() },
                    onFail = { msg -> sending = false; error = msg },
                ) { nav.repo.replyReview(r.id, text.trim()) }
            }) { Text(if (sending) "Sending…" else "Send") }
        },
        dismissButton = { TextButton(enabled = !sending, onClick = onDismiss) { Text("Cancel") } },
    )
}

/** Type a review that came by email, WhatsApp or a call. */
@Composable
private fun AddReviewDialog(nav: Nav, startProduct: String, onDismiss: () -> Unit) {
    var product by remember { mutableStateOf(startProduct) }
    var author by remember { mutableStateOf("") }
    var rating by remember { mutableIntStateOf(5) }
    var body by remember { mutableStateOf("") }
    var sending by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    AlertDialog(
        onDismissRequest = { if (!sending) onDismiss() },
        title = { Text("Add a review") },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                ChoiceRow(Products.names.keys.map { it to Products.short(it) }, product) { product = it }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    (1..5).forEach { i ->
                        Text(
                            "★", fontSize = 28.sp, color = if (i <= rating) Star else ampli().faint,
                            modifier = Modifier.clip(RoundedCornerShape(8.dp)).clickable { rating = i }.padding(horizontal = 3.dp),
                        )
                    }
                }
                OutlinedTextField(
                    author, { author = it.take(120) }, label = { Text("Name") }, singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                OutlinedTextField(
                    body, { body = it.take(4000); error = null }, label = { Text("What they said") }, minLines = 3,
                    keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Sentences),
                    modifier = Modifier.fillMaxWidth(),
                )
                error?.let { Text(it, color = ampli().bad, fontSize = 12.sp) }
            }
        },
        confirmButton = {
            TextButton(enabled = body.isNotBlank() && !sending, onClick = {
                sending = true
                nav.send(
                    done = "Review added",
                    onOk = { sending = false; onDismiss() },
                    onFail = { msg -> sending = false; error = msg },
                ) { nav.repo.addReview(product, author.trim(), rating, body.trim()) }
            }) { Text(if (sending) "Saving…" else "Add") }
        },
        dismissButton = { TextButton(enabled = !sending, onClick = onDismiss) { Text("Cancel") } },
    )
}
