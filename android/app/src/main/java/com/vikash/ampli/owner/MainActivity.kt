package com.vikash.ampli.owner

import android.os.Build
import android.os.Bundle
import android.os.SystemClock
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricManager.Authenticators.BIOMETRIC_STRONG
import androidx.biometric.BiometricManager.Authenticators.BIOMETRIC_WEAK
import androidx.biometric.BiometricManager.Authenticators.DEVICE_CREDENTIAL
import androidx.biometric.BiometricPrompt
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.ContextCompat
import androidx.fragment.app.FragmentActivity
import com.vikash.ampli.owner.data.LocalRepo
import com.vikash.ampli.owner.data.RemoteRepo
import com.vikash.ampli.owner.data.Settings
import com.vikash.ampli.owner.ui.OwnerApp
import com.vikash.ampli.owner.ui.OwnerTheme

/**
 * FragmentActivity (a ComponentActivity) so BiometricPrompt can attach its fragment; setContent works as before.
 * When a server is configured, the app asks for fingerprint, face or the screen lock before showing its data,
 * and again after five minutes in the background.
 */
class MainActivity : FragmentActivity() {
    private lateinit var settings: Settings
    private var unlocked by mutableStateOf(false)
    private var promptShowing = false
    private var stoppedAt = 0L
    private lateinit var prompt: BiometricPrompt

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        settings = Settings(this)
        unlocked = savedInstanceState?.getBoolean(KEY_UNLOCKED) ?: false
        prompt = BiometricPrompt(
            this,
            ContextCompat.getMainExecutor(this),
            object : BiometricPrompt.AuthenticationCallback() {
                override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) {
                    promptShowing = false
                    unlocked = true
                }

                override fun onAuthenticationError(errorCode: Int, errString: CharSequence) {
                    // Cancelled or too many tries: stay on the lock screen, which has an Unlock button.
                    promptShowing = false
                }
            },
        )
        setContent {
            // Bumped when Settings are saved, so the app switches between phone-only and server mode.
            var version by remember { mutableIntStateOf(0) }
            val repo = remember(version) {
                if (settings.serverUrl.isNotBlank() && settings.token.isNotBlank()) RemoteRepo(settings.serverUrl, settings.token)
                else LocalRepo(applicationContext)
            }
            OwnerTheme {
                if (unlocked || !needsLock()) {
                    // Whoever just saved Settings is already in the app: don't lock them out mid-session.
                    OwnerApp(repo) { unlocked = true; version++ }
                } else {
                    LockScreen { showPrompt() }
                }
            }
        }
    }

    override fun onStart() {
        super.onStart()
        if (stoppedAt != 0L && SystemClock.elapsedRealtime() - stoppedAt > RELOCK_AFTER_MS) unlocked = false
        stoppedAt = 0L
    }

    override fun onResume() {
        super.onResume()
        if (!unlocked && needsLock()) showPrompt()
    }

    override fun onStop() {
        super.onStop()
        if (!isChangingConfigurations) stoppedAt = SystemClock.elapsedRealtime()
    }

    override fun onSaveInstanceState(outState: Bundle) {
        super.onSaveInstanceState(outState)
        outState.putBoolean(KEY_UNLOCKED, unlocked)
    }

    /** Fingerprint/face where strong biometrics are allowed with the screen lock (Android 11+), else weak + screen lock. */
    private fun authenticators(): Int =
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) BIOMETRIC_STRONG or DEVICE_CREDENTIAL
        else BIOMETRIC_WEAK or DEVICE_CREDENTIAL

    /** Locked only with a server, the setting on, and a phone that has a fingerprint, face or screen lock. */
    private fun needsLock(): Boolean {
        if (!settings.lock) return false
        if (settings.serverUrl.isBlank() || settings.token.isBlank()) return false
        return BiometricManager.from(this).canAuthenticate(authenticators()) == BiometricManager.BIOMETRIC_SUCCESS
    }

    private fun showPrompt() {
        if (promptShowing) return
        promptShowing = true
        val info = BiometricPrompt.PromptInfo.Builder()
            .setTitle("Unlock Ampli Owner")
            .setSubtitle("Your business numbers and ad accounts")
            .setAllowedAuthenticators(authenticators())
            .build()
        prompt.authenticate(info)
    }

    private companion object {
        const val KEY_UNLOCKED = "unlocked"
        const val RELOCK_AFTER_MS = 5 * 60 * 1000L
    }
}

@Composable
private fun LockScreen(onUnlock: () -> Unit) {
    Column(
        Modifier.fillMaxSize().background(MaterialTheme.colorScheme.background).padding(32.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Icon(Icons.Outlined.Lock, contentDescription = null, modifier = Modifier.size(48.dp), tint = MaterialTheme.colorScheme.primary)
        Text(
            "Ampli Owner is locked", fontSize = 20.sp, fontWeight = FontWeight.ExtraBold,
            color = MaterialTheme.colorScheme.onBackground, modifier = Modifier.padding(top = 16.dp),
        )
        Text(
            "Use your fingerprint, face or screen lock to see your numbers.",
            textAlign = TextAlign.Center, color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(top = 6.dp),
        )
        Button(onClick = onUnlock, modifier = Modifier.padding(top = 20.dp)) { Text("Unlock") }
    }
}
