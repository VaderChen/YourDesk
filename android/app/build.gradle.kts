plugins { id("com.android.application") }

android { namespace = "com.yourdesk.android"; compileSdk = 35
    compileOptions { sourceCompatibility = JavaVersion.VERSION_17; targetCompatibility = JavaVersion.VERSION_17 }
    defaultConfig {
        applicationId = "com.yourdesk.android"
        minSdk = 26
        targetSdk = 35
        versionCode = 1
        versionName = "0.1.0"
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        // 目前產品只支援 64 位元 ARM Android 手機；排除其他 ABI 的 native library。
        ndk { abiFilters.add("arm64-v8a") }
    }
}
dependencies {
    androidTestImplementation("androidx.test:runner:1.6.2")
    androidTestImplementation("androidx.test.ext:junit:1.2.1")
    implementation(files("../libs/androidcore.aar"))
    implementation("androidx.core:core-ktx:1.16.0")
    implementation("androidx.activity:activity:1.10.1")
    implementation("androidx.lifecycle:lifecycle-runtime:2.9.2")
    implementation("androidx.webkit:webkit:1.14.0")
    // 預編譯 FFmpeg 共享庫；APK 僅輸出目前支援的 arm64-v8a。
    // 先作 fallback runtime 與能力探測，實際即時影格仍優先使用 Android 硬解。
    implementation("com.mrljdx:ffmpeg-kit-full:6.1.4")
}
