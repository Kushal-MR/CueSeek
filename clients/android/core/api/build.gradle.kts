import javax.inject.Inject

plugins {
    alias(libs.plugins.kotlin.jvm)
    alias(libs.plugins.kotlin.serialization)
    alias(libs.plugins.openapi.generator)
}

// Also plain Kotlin/JVM. Retrofit, OkHttp and kotlinx.serialization are JVM libraries,
// so the transport layer's tests run on the JVM against MockWebServer in milliseconds
// rather than on a device or under Robolectric.
java {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
}

kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17)
    }
}

// ---------------------------------------------------------------------------------------
// Generation from the contract.
//
// Types are generated; calls are not (ADR-0004 Amendment 3). The generated output is
// committed, so the tree builds without a codegen toolchain and a contract change shows up
// as a reviewable diff (ADR-0009). CI runs `generateApiTypes` and fails on any diff, the
// same gate the Go server has.
//
// `dateLibrary = "string"` on purpose: timestamps stay as the strings the wire carries and
// are parsed in the hand-written mapper. The generator's date types would otherwise become
// part of the shape this module has to hide.
// ---------------------------------------------------------------------------------------
private val wirePackage = "dev.cueseek.core.api.wire"
private val wirePath = wirePackage.replace('.', '/')

openApiGenerate {
    generatorName.set("kotlin")
    inputSpec.set(rootProject.layout.projectDirectory.file("../../api/openapi.yaml").asFile.path)
    outputDir.set(layout.buildDirectory.dir("generated/openapi").map { it.asFile.path })
    packageName.set(wirePackage)
    modelPackage.set(wirePackage)
    // Models only. The API classes this generator would emit are the part we are
    // deliberately writing by hand.
    globalProperties.set(mapOf("models" to "", "modelDocs" to "false"))
    typeMappings.set(
        mapOf(
            "HealthStatus" to "kotlin.String",
            "ActionRisk" to "kotlin.String",
            "ActionStatus" to "kotlin.String",
            "StreamEventType" to "kotlin.String",
            "Platform" to "kotlin.String",
            "Scope" to "kotlin.String",
        )
    )
    configOptions.set(
        mapOf(
            "serializationLibrary" to "kotlinx_serialization",
            "dateLibrary" to "string",
            // Version skew is permanent, not transitional (ADR-0007): a client will meet
            // enum values that did not exist when it was built. Without a fallback case,
            // one unrecognised `risk` fails the whole response rather than one field.
            "enumUnknownDefaultCase" to "true",
        )
    )
}

/**
 * Copies the generated models into the source tree.
 *
 * The destination is deliberately `@Internal` rather than `@OutputDirectory`. Declaring a
 * directory that `compileKotlin` also reads as a task output makes Gradle demand an
 * explicit dependency between the two, which would run the generator on every build and
 * defeat the point of committing its output.
 */
abstract class SyncGeneratedTypes @Inject constructor(
    private val fs: FileSystemOperations,
) : DefaultTask() {

    @get:InputDirectory
    abstract val source: DirectoryProperty

    @get:Internal
    abstract val destination: DirectoryProperty

    @TaskAction
    fun sync() {
        fs.sync {
            from(source)
            into(destination)
        }
    }
}

val generateApiTypes by tasks.registering(SyncGeneratedTypes::class) {
    group = "build"
    description = "Regenerates the wire types from api/openapi.yaml into the source tree."
    dependsOn(tasks.openApiGenerate)
    source.set(layout.buildDirectory.dir("generated/openapi/src/main/kotlin/$wirePath"))
    destination.set(layout.projectDirectory.dir("src/main/kotlin/$wirePath"))
}

dependencies {
    api(project(":core:model"))

    implementation(libs.kotlinx.serialization.json)
    implementation(libs.kotlinx.coroutines.core)
    implementation(libs.okhttp)
    implementation(libs.okhttp.sse)
    implementation(libs.retrofit)
    implementation(libs.retrofit.converter.kotlinx.serialization)

    testImplementation(libs.junit)
    testImplementation(libs.okhttp.mockwebserver)
    testImplementation(libs.kotlinx.coroutines.test)
}
