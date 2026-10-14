package com.vikash.ampli.owner.ui

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.outlined.Apps
import androidx.compose.material.icons.outlined.BarChart
import androidx.compose.material.icons.outlined.Checklist
import androidx.compose.material.icons.outlined.Home
import androidx.compose.material.icons.outlined.Inbox
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.getValue
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.dp
import com.vikash.ampli.owner.data.Repo
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch

sealed interface Route {
    data class Ticket(val id: Long) : Route
    data class Product(val id: String) : Route
    data object Checklist : Route
    data object Todos : Route
    data object Guides : Route
    data class Guide(val id: String) : Route
    data object Settings : Route
}

enum class Tab(val label: String, val icon: ImageVector) {
    Home("Home", Icons.Outlined.Home),
    Inbox("Inbox", Icons.Outlined.Inbox),
    Products("Products", Icons.Outlined.Apps),
    Money("Money", Icons.Outlined.BarChart),
    Plan("Plan", Icons.Outlined.Checklist),
}

/** Shared by every screen: the data source, navigation, a refresh counter and a safe way to run writes. */
class Nav(
    val repo: Repo,
    private val scope: CoroutineScope,
    private val snackbar: SnackbarHostState,
) {
    var tab by mutableStateOf(Tab.Home)
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
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun OwnerApp(repo: Repo, onSettingsSaved: () -> Unit) {
    val snackbar = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()
    val nav = remember(repo) { Nav(repo, scope, snackbar) }
    val top = nav.stack.lastOrNull()
    BackHandler(enabled = top != null) { nav.back() }

    val title = when (top) {
        is Route.Ticket -> "Ticket"
        is Route.Product -> com.vikash.ampli.owner.data.Products.name(top.id)
        Route.Checklist -> "Launch checklist"
        Route.Todos -> "To-do"
        Route.Guides -> "Guides"
        is Route.Guide -> "Guide"
        Route.Settings -> "Settings"
        null -> if (nav.tab == Tab.Home) "Ampli Tech" else nav.tab.label
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(title) },
                navigationIcon = {
                    if (top != null) IconButton(onClick = { nav.back() }) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                    }
                },
                actions = {
                    if (top == null) IconButton(onClick = { nav.go(Route.Settings) }) {
                        Icon(Icons.Outlined.Settings, contentDescription = "Settings")
                    }
                },
            )
        },
        bottomBar = {
            NavigationBar {
                Tab.entries.forEach { t ->
                    NavigationBarItem(
                        selected = top == null && nav.tab == t,
                        onClick = { nav.stack.clear(); nav.tab = t },
                        icon = { Icon(t.icon, contentDescription = null) },
                        label = { Text(t.label) },
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
            Route.Checklist -> ChecklistScreen(nav, m)
            Route.Todos -> TodosScreen(nav, m)
            Route.Guides -> GuidesScreen(nav, m)
            is Route.Guide -> GuideScreen(nav, top.id, m)
            Route.Settings -> SettingsScreen(nav, m, onSettingsSaved)
            null -> when (nav.tab) {
                Tab.Home -> HomeScreen(nav, m)
                Tab.Inbox -> InboxScreen(nav, m)
                Tab.Products -> ProductsScreen(nav, m)
                Tab.Money -> MoneyScreen(nav, m)
                Tab.Plan -> PlanScreen(nav, m)
            }
        }
    }
}

/** The standard scrolling page used by every screen. */
@Composable
fun Page(modifier: Modifier, content: LazyListScope.() -> Unit) {
    LazyColumn(
        modifier = modifier,
        contentPadding = PaddingValues(start = 16.dp, end = 16.dp, top = 4.dp, bottom = 96.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
        content = content,
    )
}
