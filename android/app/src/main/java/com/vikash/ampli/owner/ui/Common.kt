package com.vikash.ampli.owner.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/** Loads [load] whenever [key] changes and shows a spinner, an error with retry, or [content]. */
@Composable
fun <T> Loadable(key: Any, load: suspend () -> T, onRetry: () -> Unit, content: @Composable (T) -> Unit) {
    val state by produceState<Result<T>?>(initialValue = null, key) {
        value = null
        value = runCatching { load() }
    }
    when (val r = state) {
        null -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
        else -> r.fold(
            onSuccess = { content(it) },
            onFailure = { e ->
                Column(
                    Modifier.fillMaxSize().padding(24.dp),
                    verticalArrangement = Arrangement.Center,
                    horizontalAlignment = Alignment.CenterHorizontally,
                ) {
                    Text("Couldn't load this", style = MaterialTheme.typography.titleMedium)
                    Text(e.message ?: e.javaClass.simpleName, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Button(onClick = onRetry, modifier = Modifier.padding(top = 12.dp)) { Text("Try again") }
                }
            },
        )
    }
}

@Composable
fun Section(title: String) {
    Text(
        title.uppercase(),
        style = MaterialTheme.typography.labelMedium.copy(letterSpacing = 1.sp, fontWeight = FontWeight.SemiBold),
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(start = 4.dp, top = 8.dp),
    )
}

@Composable
fun OwnerCard(modifier: Modifier = Modifier, onClick: (() -> Unit)? = null, content: @Composable ColumnScope.() -> Unit) {
    val colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainer)
    val shape = RoundedCornerShape(18.dp)
    if (onClick != null) {
        Card(onClick = onClick, modifier = modifier.fillMaxWidth(), shape = shape, colors = colors) {
            Column(Modifier.padding(14.dp), content = content)
        }
    } else {
        Card(modifier = modifier.fillMaxWidth(), shape = shape, colors = colors) {
            Column(Modifier.padding(14.dp), content = content)
        }
    }
}

@Composable
fun Dot(color: Color, size: Int = 10) {
    Box(Modifier.size(size.dp).background(color, CircleShape))
}

@Composable
fun Pill(text: String, color: Color) {
    Surface(color = color.copy(alpha = 0.14f), shape = RoundedCornerShape(50)) {
        Text(
            text, color = color, style = MaterialTheme.typography.labelMedium,
            modifier = Modifier.padding(horizontal = 9.dp, vertical = 3.dp),
        )
    }
}

@Composable
fun Kpi(label: String, value: String, sub: String, modifier: Modifier = Modifier) {
    OwnerCard(modifier) {
        Text(label, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Text(value, style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.SemiBold)
        Text(sub, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
fun KpiRow(vararg kpis: Triple<String, String, String>) {
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
        kpis.forEach { (l, v, s) -> Kpi(l, v, s, Modifier.weight(1f)) }
    }
}

val kindLabel = mapOf("bug" to "Bug", "feature" to "Feature", "support" to "Support", "review" to "Review", "feedback" to "Feedback")
fun kindColor(kind: String) = when (kind) {
    "bug" -> Bad
    "feature" -> Violet
    "support" -> Warn
    else -> Good
}

val statusLabel = linkedMapOf(
    "new" to "New", "waiting_on_owner" to "Waiting on me", "waiting_on_user" to "Waiting on user",
    "planned" to "Planned", "in_progress" to "In progress", "fixed" to "Fixed", "closed" to "Closed",
)
