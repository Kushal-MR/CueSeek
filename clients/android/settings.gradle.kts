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
// The `core:*` modules are real modules because M4 shares :core:model and :core:design
// with the Wear client. Feature areas are not shared with anything, so they stay packages
// and do not pay for a convention-plugin build.
include(":app")
include(":core:model")
include(":core:api")
include(":core:data")
include(":core:design")
 