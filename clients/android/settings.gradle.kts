pluginManagement {
    repositories {
        google {
            content {
                includeGroupByRegex("com\\.android.*")
                includeGroupByRegex("com\\.google.*")
                includeGroupByRegex("androidx.*")
            }
        }
        mavenCentral()
        gradlePluginPortal()
    }
}
dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}

rootProject.name = "CueSeek"

// The dependency direction is enforced structurally, not by convention:
//
//   :core:model   ← nothing. Pure Kotlin/JVM: it cannot reach the Android framework.
//   :core:api     ← :core:model. Pure Kotlin/JVM, so its transport tests need no device.
//   :core:data    ← :core:api, :core:model. Android — Keystore, DataStore, lifecycle.
//   :core:design  ← :core:model. Android — Compose tokens and catalogue, shared with Wear.
//   :app          ← all four. Feature areas are packages here until M3 (ADR-0013).
//
// The `core:*` modules are real modules because M5 shares :core:model and :core:design
// with the Wear client. Feature areas are not shared with anything, so they stay packages
// and do not pay for a convention-plugin build.
include(":app")
include(":core:model")
include(":core:api")
include(":core:data")
include(":core:design")

// The Wear client lives at clients/wear/ (ADR-0009's layout) but is a project in THIS
// build rather than a build of its own.
//
// Sharing is the whole reason. :core:model and :core:api are shared with the watch, and
// two separate Gradle builds cannot share a project — they would need a composite build
// with dependency substitution, which is a second build system to understand before
// reading any application code. That is the cost ADR-0013 already refused to pay for
// convention plugins.
//
// So the build root stays clients/android and reaches one project outside it. That is
// legal Gradle and slightly odd to read; the alternative is moving settings.gradle.kts up
// to clients/ and renaming every path in CI and the release workflow, which M4.10 has
// just finished verifying. The trigger to revisit is a third client, or the day this
// build acquires convention plugins anyway.
include(":wear")
project(":wear").projectDir = file("../wear/app")
 