package com.vikash.ampli.owner.ui

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.background
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
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
    Loadable(
        key = nav.tick to product,
        load = { Pair(product, nav.repo.reviews(product)) },
        onRetry = nav::refresh,
        keepStale = true,
    ) { loaded ->
        ReviewsContent(loaded.first, loaded.second, modifier) { replying = it }
    }
    replying?.let { r ->
        ReplyDialog(r, onDismiss = { replying = null }) { text ->
            replying = null
            nav.run("Reply sent") { nav.repo.replyReview(r.id, text) }
        }
    }
}

@Composable
private fun ReviewsContent(key: String, data: ReviewsResponse, modifier: Modifier, onReply: (Review) -> Unit) {
    val a = ampli()
    val s = data.summary
    ScrollPage(modifier, key) {
        var n = 0

        AmpliCard(Modifier.rise(key, n++)) {
            CardTitle("How people feel", "${grouped(s.total)} reviews")
            SentimentGauge(
                listOf(s.positive.toFloat() to a.good, s.mixed.toFloat() to a.warn, s.negative.toFloat() to a.bad),
                key,
            )
            Row(Modifier.fillMaxWidth().padding(top = 8.dp)) {
                Text("😊 ${Math.round(s.positive)}% positive", fontSize = 12.sp, fontWeight = FontWeight.Bold, color = a.good)
                Spacer(Modifier.weight(1f))
                Text("${Math.round(s.mixed)}% mixed", fontSize = 12.sp, fontWeight = FontWeight.Bold, color = a.warn)
                Spacer(Modifier.weight(1f))
                Text("${Math.round(s.negative)}% negative 😞", fontSize = 12.sp, fontWeight = FontWeight.Bold, color = a.bad)
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
                    withStyle(SpanStyle(color = a.line)) { append("★".repeat(5 - stars)) }
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

@Composable
private fun ReplyDialog(r: Review, onDismiss: () -> Unit, onSend: (String) -> Unit) {
    var text by remember { mutableStateOf(r.reply) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Reply to ${r.author.ifBlank { "this review" }}") },
        text = {
            Column {
                Text(r.body, fontSize = 12.5.sp, color = ampli().muted, maxLines = 4, overflow = TextOverflow.Ellipsis)
                Spacer(Modifier.height(10.dp))
                OutlinedTextField(
                    value = text,
                    onValueChange = { text = it },
                    label = { Text("Your reply (posts to Google Play)") },
                    minLines = 3,
                    keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Sentences),
                    modifier = Modifier.fillMaxWidth(),
                )
            }
        },
        confirmButton = {
            TextButton(enabled = text.isNotBlank(), onClick = { onSend(text.trim()) }) { Text("Send") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}
