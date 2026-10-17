package com.vikash.ampli.owner.ui

import androidx.activity.compose.BackHandler
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.outlined.Campaign
import androidx.compose.material.icons.outlined.Checklist
import androidx.compose.material.icons.outlined.Dashboard
import androidx.compose.material.icons.outlined.Inbox
import androidx.compose.material.icons.outlined.Refresh
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material.icons.outlined.StarOutline
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.vikash.ampli.owner.data.Products
import com.vikash.ampli.owner.data.Repo
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch
import java.time.LocalTime

sealed interface Route {
    data class Ticket(val id: Long) : Route
    data class Product(val id: String) : Route
    data object Roadmap : Route
    data object Checklist : Route
    data object Todos : Route
    data object Guides : Route
    data class Guide(val id: String) : Route
    data object Settings : Route
}

enum class Tab(val label: String, val icon: ImageVector, val productScoped: Boolean) {
    Dashboard("Dashboard", Icons.Outlined.Dashboard, true),
    Ads("Ads", Icons.Outlined.Campaign, true),
    Reviews("Reviews", Icons.Outlined.StarOutline, true),
    Inbox("Inbox", Icons.Outlined.Inbox, false),
    Plan("Plan", Icons.Outlined.Checklist, false),
}

/** Shared by every screen: the data source, navigation, a refresh counter and a safe way to run writes. */
class Nav(
    val repo: Repo,
    private val scope: CoroutineScope,
    private val snackbar: SnackbarHostState,
) {
    var tab by mutableStateOf(Tab.Dashboard)
    /** "all", "mp3", "carhub" or "naukri": picked in the switcher, used by Dashboard, Ads and Reviews. */
    var product by mutableStateOf("all")
    val stack = mutableStateListOf<Route>()
    var tick by mutableIntStateOf(0)
        private set

    fun go(r: Route) { stack.add(r) }
    fun back() { if (stack.isNotEmpty()) stack.removeAt(stack.lastIndex) }
    fun refresh() { tick++ }

    /** Runs a write, refreshes on success, and shows the error otherwise. */
    fun run(done: String? = null, block: suspend () -> Unit) {
        scope.launch {
            runCatching { block() }
                .onSuccess { refresh(); done?.let { snackbar.showSnackbar(it) } }
                .onFailure { snackbar.showSnackbar(it.message ?: "Something went wrong") }
        }
    }

    /** Shows a short message without refreshing. */
    fun toast(message: String) {
        scope.launch { snackbar.showSnackbar(message) }
    }
}

private fun greeting(): String {
    val h = LocalTime.now().hour
    val part = if (h < 12) "Good morning" else if (h < 17) "Good afternoon" else "Good evening"
    return "$part, Vikash"
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun OwnerApp(repo: Repo, onSettingsSaved: () -> Unit) {
    val snackbar = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()
    val nav = remember(repo) { Nav(repo, scope, snackbar) }
    val top = nav.stack.lastOrNull()
    BackHandler(enabled = top != null) { nav.back() }

    // The selected product's colour becomes the accent (and Material primary) for everything below.
    val palette = ampli()
    val accentColor by animateColorAsState(palette.product(nav.product), tween(350), label = "accent")
    val base = MaterialTheme.colorScheme

    MaterialTheme(colorScheme = base.copy(primary = accentColor, onPrimary = Color.White)) {
        CompositionLocalProvider(LocalAccent provides accentColor) {
            val title = when (top) {
                is Route.Ticket -> "Ticket"
                is Route.Product -> Products.name(top.id)
                Route.Roadmap -> "Built vs left"
                Route.Checklist -> "Launch checklist"
                Route.Todos -> "To-do"
                Route.Guides -> "Guides"
                is Route.Guide -> "Guide"
                Route.Settings -> "Settings"
                null -> null
            }

            Scaffold(
                topBar = {
                    Column(Modifier.background(MaterialTheme.colorScheme.background)) {
                        TopAppBar(
                            title = {
                                if (title != null) {
                                    Text(title)
                                } else {
                                    Column {
                                        Text("Ampli Owner", fontSize = 17.sp, fontWeight = FontWeight.ExtraBold)
                                        Text(greeting(), fontSize = 11.sp, color = palette.muted)
                                    }
                                }
                            },
                            navigationIcon = {
                                if (top != null) IconButton(onClick = { nav.back() }) {
                                    Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                                }
                            },
                            actions = {
                                if (!repo.isRemote) SampleChip()
                                if (top == null && nav.tab == Tab.Ads) IconButton(onClick = {
                                    nav.run("Ad numbers updated") { nav.repo.syncAds() }
                                }) {
                                    Icon(Icons.Outlined.Refresh, contentDescription = "Sync ad numbers")
                                }
                                if (top == null) IconButton(onClick = { nav.go(Route.Settings) }) {
                                    Icon(Icons.Outlined.Settings, contentDescription = "Settings")
                                }
                            },
                            colors = TopAppBarDefaults.topAppBarColors(
                                containerColor = MaterialTheme.colorScheme.background,
                                scrolledContainerColor = MaterialTheme.colorScheme.background,
                            ),
                        )
                        if (top == null && nav.tab.productScoped) {
                            ProductSwitcher(
                                selected = nav.product,
                                onSelect = { nav.product = it },
                                modifier = Modifier.padding(start = 16.dp, end = 16.dp, bottom = 8.dp),
                            )
                        }
                    }
                },
                bottomBar = {
                    NavigationBar(containerColor = MaterialTheme.colorScheme.surfaceContainer) {
                        Tab.entries.forEach { t ->
                            NavigationBarItem(
                                selected = top == null && nav.tab == t,
                                onClick = { nav.stack.clear(); nav.tab = t },
                                icon = { Icon(t.icon, contentDescription = null) },
                                label = { Text(t.label, maxLines = 1) },
                                colors = NavigationBarItemDefaults.colors(
                                    indicatorColor = accentColor.copy(alpha = 0.18f),
                                    selectedIconColor = MaterialTheme.colorScheme.onSurface,
                                    selectedTextColor = MaterialTheme.colorScheme.onSurface,
                                ),
                            )
                        }
                    }
                },
                snackbarHost = { SnackbarHost(snackbar) },
            ) { pad ->
                val m = Modifier.padding(pad)
                when (top) {
                    is Route.Ticket -> TicketScreen(nav, top.id, m)
                    is Route.Product -> ProductScreen(nav, top.id, m)
                    Route.Roadmap -> ProductsScreen(nav, m)
                    Route.Checklist -> ChecklistScreen(nav, m)
                    Route.Todos -> TodosScreen(nav, m)
                    Route.Guides -> GuidesScreen(nav, m)
                    is Route.Guide -> GuideScreen(nav, top.id, m)
                    Route.Settings -> SettingsScreen(nav, m, onSettingsSaved)
                    null -> when (nav.tab) {
                        Tab.Dashboard -> DashboardScreen(nav, m)
                        Tab.Ads -> AdsScreen(nav, m)
                        Tab.Reviews -> ReviewsScreen(nav, m)
                        Tab.Inbox -> InboxScreen(nav, m)
                        Tab.Plan -> PlanScreen(nav, m)
                    }
                }
            }
        }
    }
}

/** The standard scrolling page used by the list screens. */
@Composable
fun Page(modifier: Modifier, content: LazyListScope.() -> Unit) {
    LazyColumn(
        modifier = modifier,
        contentPadding = PaddingValues(start = 16.dp, end = 16.dp, top = 4.dp, bottom = 96.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
        content = content,
    )
}
