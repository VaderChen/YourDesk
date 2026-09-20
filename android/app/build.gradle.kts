plugins { id("com.android.application") }

// Source changes must not silently ship the old prebuilt Go core.
val verifyAndroidCore by tasks.registering(Exec::class) {
    workingDir(rootProject.projectDir.parentFile)
    commandLine(providers.gradleProperty("python").getOrElse("python3"), "scripts/android_core.py", "verify")
}
tasks.register<Exec>("buildAndroidCore") {
    workingDir(rootProject.projectDir.parentFile)
    commandLine(providers.gradleProperty("python").getOrElse("python3"), "scripts/android_core.py", "build")
}
verifyAndroidCore.configure { mustRunAfter("buildAndroidCore") }
tasks.named("preBuild") { dependsOn(verifyAndroidCore) }

android { namespace = "com.yourdesk.android"; compileSdk = 35
    compileOptions { sourceCompatibility = JavaVersion.VERSION_17; targetCompatibility = JavaVersion.VERSION_17 }
    defaultConfig {
        applicationId = "com.yourdesk.android"
        minSdk = 26
        targetSdk = 35
        versionCode = 29824441
        versionName = "0.26.0915 build 1801"
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        // 目前產品只支援 64 位元 ARM Android 手機；排除其他 ABI 的 native library。
        ndk { abiFilters.add("arm64-v8a") }
    }

    // 預覽版 APK 使用可辨識的產品與版本檔名。
}

tasks.register("releasePreviewApk") {
    dependsOn("verifyReleaseNativeLibraries")
    doLast {
        copy {
            from(layout.buildDirectory.file("outputs/apk/release/app-release-unsigned.apk"))
            into(layout.buildDirectory.dir("outputs/apk/release"))
            rename { "YourDesk-0.26.0915-build-1801-preview.apk" }
        }
    }
}

// Inspect the final APK too: third-party FFmpeg/MLKit libraries are not in the Go AAR.
val verifyReleaseNativeLibraries by tasks.registering(Exec::class) {
    dependsOn("assembleRelease")
    workingDir(rootProject.projectDir.parentFile)
    commandLine(providers.gradleProperty("python").getOrElse("python3"), "scripts/android_core.py", "verify-apk",
        "--apk", layout.buildDirectory.file("outputs/apk/release/app-release-unsigned.apk").get().asFile.path)
}
// AGP creates variant assemble tasks after project evaluation.
tasks.configureEach {
    if (name == "assembleRelease") finalizedBy(verifyReleaseNativeLibraries)
}
dependencies {
    androidTestImplementation("androidx.test:runner:1.6.2")
    androidTestImplementation("androidx.test.ext:junit:1.2.1")
    implementation(files("../libs/androidcore.aar"))
    implementation("androidx.core:core-ktx:1.16.0")
    implementation("androidx.activity:activity:1.10.1")
    implementation("androidx.lifecycle:lifecycle-runtime:2.9.2")
    implementation("androidx.lifecycle:lifecycle-process:2.9.2")
    implementation("androidx.webkit:webkit:1.14.0")
    // 預編譯 FFmpeg 共享庫；APK 僅輸出目前支援的 arm64-v8a。
    // 先作 fallback runtime 與能力探測，實際即時影格仍優先使用 Android 硬解。
    implementation("com.mrljdx:ffmpeg-kit-full:6.1.4")
    implementation("androidx.camera:camera-camera2:1.4.2")
    implementation("androidx.camera:camera-lifecycle:1.4.2")
    implementation("androidx.camera:camera-view:1.4.2")
    implementation("com.google.mlkit:barcode-scanning:17.3.0")
}
