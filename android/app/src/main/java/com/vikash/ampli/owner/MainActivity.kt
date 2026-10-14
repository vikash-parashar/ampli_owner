package com.vikash.ampli.owner

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import com.vikash.ampli.owner.data.LocalRepo
import com.vikash.ampli.owner.data.RemoteRepo
import com.vikash.ampli.owner.data.Settings
import com.vikash.ampli.owner.ui.OwnerApp
import com.vikash.ampli.owner.ui.OwnerTheme

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val settings = Settings(this)
        setContent {
            // Bumped when Settings are saved, so the app switches between phone-only and server mode.
            var version by remember { mutableIntStateOf(0) }
            val repo = remember(version) {
                if (settings.serverUrl.isNotBlank() && settings.token.isNotBlank()) RemoteRepo(settings.serverUrl, settings.token)
                else LocalRepo(applicationContext)
            }
            OwnerTheme { OwnerApp(repo) { version++ } }
        }
    }
}
