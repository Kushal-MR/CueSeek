plugins {
    alias(libs.plugins.kotlin.jvm)
}

// Deliberately a plain Kotlin/JVM module. The domain vocabulary the UI speaks must not
// be able to reach the Android framework, and the compiler is a better enforcement of
// that than a review comment. It also mirrors agent/internal/domain, which depends on
// nothing for the same reason.
java {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
}

kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17)
    }
}

dependencies {
    testImplementation(libs.junit)
}
