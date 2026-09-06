plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
}

// ---------------------------------------------------------------- version
//
// Duplicated from the phone's build file rather than factored out, which is ADR-0013's
// standing trade: at this module count the duplication is smaller than the convention
// plugin that would remove it. The trigger to revisit is a third consumer, not a line
// count.
//
// The versionCode SCHEME is deliberately not settled here — M5.16 decides how a watch
// artefact and a phone artefact that share an applicationId coordinate their codes. This
// is enough to build and install.
val releaseTag: String? = providers.environmentVariable("CUESEEK_VERSION").orNull
    ?.trim()?.takeIf { it.isNotEmpty() }

fun versionNameFrom(tag: String?): String = tag?.removePrefix("v") ?: "0.0.0-dev"

fun versionCodeFrom(tag: String?): Int {
    val core = tag?.removePrefix("v")?.substringBefore('-') ?: return 1
    val parts = core.split('.').mapNotNull(String::toIntOrNull)
    if (parts.size != 3) return 1
    return parts[0] * 10000 + parts[1] * 100 + parts[2]
}

android {
    // The R class package. Deliberately different from the applicationId below: this is
    // watch code and should not be mistaken for the phone's in a stack trace.
    namespace = "dev.cueseek.wear"

    compileSdk {
        version = release(37)
    }

    defaultConfig {
        // The SAME applicationId as the phone client, and that is the decision rather than
        // an oversight.
        //
        // The Wearable Data Layer that ADR-0014's pairing handoff depends on expects the
        // phone and watch halves of one app to share an install identity; it is also how
        // Android models a multi-form-factor app, and the only shape that could ever be
        // delivered by form factor from one listing. Two devices, one identity, no
        // conflict — they are never installed side by side.
        //
        // The cost is that the two artefacts must coordinate versionCode. That is M5.16.
        applicationId = "dev.cueseek.android"

        // Wear OS 3 (API 30) is the first release with the standalone app model this
        // client assumes. The library floor is lower — Wear Compose Material 3 declares
        // minSdk 25 — but a Wear OS 2 companion app is a different product.
        //
        // Only Wear OS 5 / API 34 is actually tested; see docs/requirements.md. Stating a
        // floor below what was tested is the same stance the phone client takes at API 26.
        minSdk = 30

        // Matches what the test device runs. Raised when there is a device to raise it
        // against, not before.
        targetSdk = 34

        versionCode = versionCodeFrom(releaseTag)
        versionName = versionNameFrom(releaseTag)
    }

    buildTypes {
        debug {
            // Coexists with a release build on the same watch, exactly as on the phone.
            applicationIdSuffix = ".debug"
            versionNameSuffix = "-debug"
        }
    }

    // Matches every other module in this build. Not a toolchain declaration: the phone
    // modules compile against the JDK Gradle already runs on, and a watch module that
    // demanded its own would fail on a machine that builds the rest of the project fine.
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    buildFeatures {
        compose = true
    }
}

dependencies {
    // The claim ADR-0013 made in M2, now under test: :core:model and :core:api are plain
    // Kotlin/JVM specifically so a second consumer inherits them with no audit. If this
    // line had needed a change, that would have been the finding.
    implementation(project(":core:model"))
    implementation(project(":core:api"))

    // The design system, for its tokens: the status palette, the Plex faces, motion.
    // NOT for its components -- those are phone Material 3 and are wrong on a wrist
    // (ADR-0010). Since M5.2 that is enforced rather than requested: :core:design demoted
    // Material 3 to `implementation`, so this module cannot see it and an accidental
    // import fails the build.
    implementation(project(":core:design"))

    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.tooling.preview)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.core.ktx)

    // Wear's Material 3, not the phone's. Both are on the classpath transitively through
    // the BOM; importing androidx.compose.material3 in this module is a defect.
    implementation(libs.androidx.wear.compose.material3)
    implementation(libs.androidx.wear.compose.foundation)
    implementation(libs.androidx.wear.compose.navigation)

    debugImplementation(libs.androidx.compose.ui.tooling)

    testImplementation(libs.junit)
}
