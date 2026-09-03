plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
}

// ---------------------------------------------------------------- version
//
// Both numbers come from the release tag, through one environment variable, so that the
// APK on a phone can be traced to a tag in the repository. They were pinned at 1 / "1.0"
// until M4.7, which meant every build ever produced claimed to be the same one — the same
// defect the agent carried until M4.6, and it cost a whole debugging session there
// (M4.5b). A build that cannot say which build it is cannot answer the first question
// anybody asks about a bug.
//
// CUESEEK_VERSION is the tag, e.g. "v0.1.0". Absent for a local build, which is the
// ordinary case and must stay frictionless.
val releaseTag: String? = providers.environmentVariable("CUESEEK_VERSION").orNull
    ?.trim()?.takeIf { it.isNotEmpty() }

/** "v0.1.0" -> "0.1.0". Absent -> a name that is obviously not a release. */
fun versionNameFrom(tag: String?): String = tag?.removePrefix("v") ?: "0.0.0-dev"

/**
 * "v0.1.0" -> 100. Absent or unparseable -> 1.
 *
 * `major * 10000 + minor * 100 + patch`, which stays ordered as long as minor and patch
 * remain below 100 — comfortable for a project at this scale, and it fails loudly by
 * refusing to increase rather than silently wrapping.
 *
 * Known limitation, stated rather than discovered: a pre-release shares its versionCode
 * with the final release of the same number, so `v0.2.0-rc1` and `v0.2.0` collide and
 * Android will not upgrade one to the other. Pre-release APKs are therefore not published;
 * see the workflow.
 */
fun versionCodeFrom(tag: String?): Int {
    val core = tag?.removePrefix("v")?.substringBefore('-') ?: return 1
    val parts = core.split('.').mapNotNull(String::toIntOrNull)
    if (parts.size != 3) return 1
    return parts[0] * 10000 + parts[1] * 100 + parts[2]
}

// ---------------------------------------------------------------- signing
//
// Supplied by the environment, never by a file in the repository. The keystore is the one
// credential that cannot be rotated: losing it means no future build can upgrade an
// existing install, for anybody, ever.
//
// All four values must be present. A half-configured signing block is worse than none,
// because Gradle fails deep inside a packaging task with a message about a keystore rather
// than about configuration.
val keystorePath: String? = providers.environmentVariable("CUESEEK_KEYSTORE").orNull
    ?.trim()?.takeIf { it.isNotEmpty() }
val keystorePassword: String? = providers.environmentVariable("CUESEEK_KEYSTORE_PASSWORD").orNull
val keyAliasName: String? = providers.environmentVariable("CUESEEK_KEY_ALIAS").orNull
val keyPasswordValue: String? = providers.environmentVariable("CUESEEK_KEY_PASSWORD").orNull

val canSign: Boolean =
    keystorePath != null && keystorePassword != null &&
        keyAliasName != null && keyPasswordValue != null

android {
    namespace = "dev.cueseek.android"
    compileSdk {
        version = release(37)
    }

    defaultConfig {
        applicationId = "dev.cueseek.android"
        minSdk = 26
        targetSdk = 36
        versionCode = versionCodeFrom(releaseTag)
        versionName = versionNameFrom(releaseTag)

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    signingConfigs {
        // Created only when the environment supplies all four values, so that `./gradlew
        // build` works unchanged for anyone without the keystore — which is CI on every
        // pull request, and every contributor.
        if (canSign) {
            create("release") {
                storeFile = file(keystorePath!!)
                storePassword = keystorePassword
                keyAlias = keyAliasName
                keyPassword = keyPasswordValue
            }
        }
    }

    buildTypes {
        debug {
            // The whole point of M4.7, and it lands before the signing does.
            //
            // A release-signed APK cannot install over a debug-signed one carrying the same
            // applicationId: Android refuses on the signature mismatch, and uninstalling to
            // get past it clears the credential store and unpairs the device. Giving the
            // debug build its own id means the two are different apps, so a development
            // build and the published one coexist on one phone permanently and neither
            // can displace the other.
            applicationIdSuffix = ".debug"

            // So that a screenshot of the app says which of the two it is.
            versionNameSuffix = "-debug"
        }

        release {
            // Null when the environment carries no keystore, which produces an unsigned
            // APK rather than a build failure. An unsigned artefact cannot be installed,
            // which is the correct outcome for a build nobody could have signed.
            signingConfig = signingConfigs.findByName("release")

            // Deliberately off. R8 would take a few megabytes off an APK that is already
            // small, in exchange for a class of failure that appears only in the release
            // build and only at runtime. This project does not ship what it has not run,
            // and nothing here is shipped often enough for the size to matter.
            isMinifyEnabled = false
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
        }
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    buildFeatures {
        compose = true
    }
}

// Assembly, navigation and dependency wiring only. Feature areas live here as packages
// (`pairing`, `dashboard`) rather than as modules until M3 — see ADR-0013.
dependencies {
    implementation(project(":core:model"))
    implementation(project(":core:api"))
    implementation(project(":core:data"))
    implementation(project(":core:design"))

    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.navigation.compose)
    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.graphics)
    implementation(libs.androidx.compose.ui.tooling.preview)
    implementation(libs.androidx.compose.material3)
    testImplementation(libs.junit)
    androidTestImplementation(libs.androidx.junit)
    androidTestImplementation(libs.androidx.espresso.core)
    androidTestImplementation(platform(libs.androidx.compose.bom))
    androidTestImplementation(libs.androidx.compose.ui.test.junit4)
    debugImplementation(libs.androidx.compose.ui.tooling)
    debugImplementation(libs.androidx.compose.ui.test.manifest)
}