import java.util.Base64

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
}

/** The release key from CI secrets (base64 keystore), written into the build dir; null when not set. */
fun releaseKeystore(): java.io.File? {
    val b64 = System.getenv("OWNER_KEYSTORE_BASE64")
    if (b64.isNullOrBlank()) return null
    val f = layout.buildDirectory.file("owner-release.keystore").get().asFile
    f.parentFile.mkdirs()
    f.writeBytes(Base64.getMimeDecoder().decode(b64.trim()))
    return f
}

android {
    namespace = "com.vikash.ampli.owner"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.vikash.ampli.owner"
        minSdk = 26
        targetSdk = 35
        // CI passes the run number so each test build installs as an update over the last one.
        versionCode = (System.getenv("GITHUB_RUN_NUMBER") ?: "1").toInt()
        versionName = "0.1.${System.getenv("GITHUB_RUN_NUMBER") ?: "0"}"
    }

    signingConfigs {
        // Local debug builds keep the committed test key.
        getByName("debug") {
            storeFile = file("owner-test.keystore")
            storePassword = "android"
            keyAlias = "owner"
            keyPassword = "android"
        }
        // Release: only the key from GitHub secrets (OWNER_KEYSTORE_BASE64, OWNER_KEYSTORE_PASSWORD, OWNER_KEY_ALIAS,
        // OWNER_KEY_PASSWORD). Never the committed test key: without the secrets the release APK is left unsigned
        // and CI builds the debug APK instead.
        val ks = releaseKeystore()
        if (ks != null) {
            create("release") {
                storeFile = ks
                storePassword = System.getenv("OWNER_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("OWNER_KEY_ALIAS")
                keyPassword = System.getenv("OWNER_KEY_PASSWORD")
            }
        }
    }

    buildTypes {
        debug { signingConfig = signingConfigs.getByName("debug") }
        release {
            // No R8: keeps kotlinx.serialization and Compose free of shrinker surprises; the app is small.
            isMinifyEnabled = false
            isShrinkResources = false
            isDebuggable = false
            signingConfig = signingConfigs.findByName("release")
        }
    }

    // assembleRelease runs lint "vital" checks; don't let a lint nit block the phone build.
    lint {
        checkReleaseBuilds = false
        abortOnError = false
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions { jvmTarget = "17" }
    buildFeatures { compose = true }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.biometric)
    // biometric 1.1.0 pulls an old fragment; pin a current one so FragmentActivity matches activity 1.9.
    implementation(libs.androidx.fragment)

    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.tooling.preview)
    implementation(libs.androidx.compose.material3)
    implementation(libs.androidx.compose.material.icons.extended)
    debugImplementation(libs.androidx.compose.ui.tooling)

    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.kotlinx.serialization.json)
}
