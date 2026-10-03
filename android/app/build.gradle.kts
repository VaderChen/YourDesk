import java.util.Properties

plugins { id("com.android.application") }

// 本機重新連結的衍生 AAR 保留 upstream POM，不繞過傳遞依賴或原生庫檢查。
configurations.configureEach {
    resolutionStrategy.dependencySubstitution {
        substitute(module("androidx.camera:camera-core"))
            .using(module("com.yourdesk.thirdparty:camera-core:1.6.2-arm64-16k1"))
            .because("固定官方 JNI 來源重新連結 16 KB RELRO")
    }
}
val verifyAndroidNative = tasks.register<Exec>("verifyAndroidNative") {
    workingDir(rootProject.projectDir.parentFile)
    commandLine(providers.gradleProperty("python").getOrElse("python3"), "scripts/android_native.py", "verify")
}
tasks.named("preBuild") { dependsOn(verifyAndroidNative) }

val releaseVersion = Properties().apply {
    rootProject.file("version.properties").inputStream().use { load(it) }
}
val productVersion = releaseVersion.getProperty("versionName")
val productCode = releaseVersion.getProperty("versionCode").toInt()
require(productCode > 0 && productVersion.matches(Regex("[0-9]+\\.[0-9]{2}\\.[0-9]{4} build [0-9]{4}"))) {
    "Android 發行版本格式無效"
}

val verifyAndroidCore = tasks.register<Exec>("verifyAndroidCore") {
    workingDir(rootProject.projectDir.parentFile)
    commandLine(providers.gradleProperty("python").getOrElse("python3"), "scripts/android_core.py", "verify")
}
tasks.register<Exec>("buildAndroidCore") {
    workingDir(rootProject.projectDir.parentFile)
    commandLine(providers.gradleProperty("python").getOrElse("python3"), "scripts/android_core.py", "build")
}
verifyAndroidCore.configure { mustRunAfter("buildAndroidCore") }
tasks.named("preBuild") { dependsOn(verifyAndroidCore) }

android {
    namespace = "com.yourdesk.android"
    compileSdk = 36
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    defaultConfig {
        applicationId = "com.yourdesk.android"
        minSdk = 26
        targetSdk = 36
        versionCode = productCode
        versionName = productVersion
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        ndk { abiFilters.add("arm64-v8a") }
    }
    buildTypes {
        getByName("debug") {
            // 實機 Smoke 與正式套件分開安裝，避免改寫使用者的站台與密碼。
            applicationIdSuffix = ".debug"
            versionNameSuffix = "-debug"
        }
        getByName("release") {
            isDebuggable = false
        }
    }
    androidResources { ignoreAssetsPattern = "!.git:!.svn:!.DS_Store:!*.bak:!*.bak.*:!*~" }
    sourceSets.getByName("androidTest").assets.srcDir("../tests/smoke-host/fixtures")
    lint { abortOnError = true; checkReleaseBuilds = true }
}

val verifyReleaseNativeLibraries = tasks.register<Exec>("verifyReleaseNativeLibraries") {
    dependsOn("assembleRelease")
    workingDir(rootProject.projectDir.parentFile)
    commandLine(providers.gradleProperty("python").getOrElse("python3"), "scripts/android_core.py", "verify-apk",
        "--apk", layout.buildDirectory.file("outputs/apk/release/app-release-unsigned.apk").get().asFile.path)
}
tasks.configureEach {
    if (name == "assembleRelease") finalizedBy(verifyReleaseNativeLibraries)
}

// 相容舊入口；預覽檔仍未簽章。可安裝正式檔請使用 scripts/android_release.py。
tasks.register("releasePreviewApk") {
    dependsOn(verifyReleaseNativeLibraries)
    doLast {
        copy {
            from(layout.buildDirectory.file("outputs/apk/release/app-release-unsigned.apk"))
            into(layout.buildDirectory.dir("outputs/apk/release"))
            rename { "YourDesk-${productVersion.replace(" ", "-")}-preview-unsigned.apk" }
        }
    }
}

dependencies {
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.json:json:20240303")
    androidTestImplementation("androidx.test:runner:1.6.2")
    androidTestImplementation("androidx.test.ext:junit:1.2.1")
    implementation(files("../libs/androidcore.aar"))
    implementation("androidx.core:core-ktx:1.16.0")
    implementation("androidx.activity:activity:1.10.1")
    implementation("androidx.lifecycle:lifecycle-runtime:2.9.2")
    implementation("androidx.webkit:webkit:1.14.0")
    // 播放採 MediaCodec 與 JPEG；不封裝只有版本探測用途的 FFmpeg 原生庫。
    implementation("androidx.camera:camera-camera2:1.6.2")
    implementation("androidx.camera:camera-lifecycle:1.6.2")
    implementation("com.google.mlkit:barcode-scanning:17.3.0")
}
