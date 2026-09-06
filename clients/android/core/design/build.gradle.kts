plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.paparazzi)
}

android {
    namespace = "dev.cueseek.core.design"
    compileSdk {
        version = release(37)
    }
    defaultConfig {
        minSdk = 26
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    buildFeatures {
        compose = true
    }
}

// Tokens and the component catalogue (ADR-0010). Tokens are shared with Wear in M5;
// components are not, because the same capability is deliberately rendered differently
// per form factor (ADR-0007). Paparazzi arrives with the status catalogue in P4 — there
// is nothing worth pinning a golden image of yet.
dependencies {
    // `api`, not `implementation`: composables here take HealthStatus and ActionRisk in
    // their signatures, so consumers must see those types to call them.
    api(project(":core:model"))

    implementation(platform(libs.androidx.compose.bom))
    // `implementation`, not `api`, since M5.2.
    //
    // This module is shared with the Wear client, and Wear's Material 3 is a different
    // library (ADR-0010: tokens shared, components not). While this was `api`, every
    // consumer of the design system inherited the PHONE's Material 3 on its compile
    // classpath — so "do not import androidx.compose.material3 on the watch" was a rule
    // enforced by discipline, which ADR-0013 explicitly prefers not to rely on.
    //
    // Demoting it makes the compiler enforce it instead. The phone app declares Material 3
    // for itself and is unaffected; the watch simply cannot see it.
    implementation(libs.androidx.compose.material3)
    api(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.graphics)
    implementation(libs.androidx.compose.ui.tooling.preview)
    debugImplementation(libs.androidx.compose.ui.tooling)

    testImplementation(libs.junit)
}
