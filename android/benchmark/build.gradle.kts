plugins {
    alias(libs.plugins.android.test)
}

android {
    experimentalProperties["android.experimental.self-instrumenting"] = true
    namespace = "com.autoanime.android.benchmark.test"
    compileSdk = 37
    targetProjectPath = ":app"
    buildTypes {
        create("benchmark") {
            matchingFallbacks += listOf("release")
            isMinifyEnabled = true
            signingConfig = signingConfigs.getByName("debug")
            testProguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), file("proguard-rules.pro"))
        }
    }
    defaultConfig {
        minSdk = 26
        targetSdk = 36
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}


dependencies {
    implementation("androidx.arch.core:core-runtime:2.2.0")
    implementation("androidx.concurrent:concurrent-futures:1.2.0")
    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.androidx.profileinstaller)
    implementation("androidx.startup:startup-runtime:1.2.0")
    implementation("com.google.errorprone:error_prone_annotations:2.36.0")
    implementation(libs.androidx.test.runner)
    implementation("androidx.tracing:tracing:1.2.0")
    implementation(libs.androidx.test.uiautomator)
    implementation(libs.androidx.benchmark.macro.junit4)
}
