plugins {
    alias(libs.plugins.android.library)
}

android {
    namespace = "dev.cueseek.core.data"
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
}

// The first module that genuinely needs Android: the token is sealed with an Android
// Keystore key (ADR-0013 supersedes ADR-0006's EncryptedSharedPreferences) and every
// repository is keyed by host_id from day one (ADR-0008).
dependencies {
    api(project(":core:api"))
    api(project(":core:model"))

    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.datastore.preferences)
    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.kotlinx.serialization.json)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
}
