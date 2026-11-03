plugins {
    id("autoanime.android.application")
    id("autoanime.android.compose")
    id("autoanime.hilt")
    alias(libs.plugins.kotlin.serialization)
}

android {
    namespace = "com.autoanime.android"
    defaultConfig {
        applicationId = "com.autoanime.android"
        versionCode = 1
        versionName = "0.1.0"
    }
    buildTypes {
        create("benchmark") {
            initWith(getByName("release"))
            applicationIdSuffix = ".benchmark"
            isDebuggable = false
            isProfileable = true
            isMinifyEnabled = true
            signingConfig = signingConfigs.getByName("debug")
            matchingFallbacks += listOf("release")
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"))
        }
    }
    buildFeatures { buildConfig = true }
}

dependencies {
    implementation(projects.core.model)
    implementation(projects.core.data)
    implementation(projects.core.designsystem)
    implementation(projects.feature.connect)
    implementation(projects.feature.downloads)
    implementation(projects.feature.library)
    implementation(projects.feature.player)
    implementation(projects.feature.settings)

    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.lifecycle.viewmodel.navigation3)
    implementation(libs.androidx.hilt.lifecycle.viewmodel.compose)
    implementation(libs.androidx.navigation3.runtime)
    implementation(libs.androidx.navigation3.ui)
    implementation(libs.androidx.profileinstaller)
    implementation(libs.compose.material3)
    implementation(libs.compose.material3.adaptive)
    implementation(libs.compose.material3.adaptive.navigation3)
    implementation(libs.compose.material3.adaptive.navigation.suite)
    implementation(libs.kotlinx.serialization.json)

    androidTestImplementation(libs.androidx.test.ext.junit)
    androidTestImplementation(libs.androidx.test.core.ktx)
    androidTestImplementation(libs.androidx.test.runner)
    androidTestImplementation(libs.compose.ui.test.junit4)
    androidTestImplementation(libs.okhttp.mockwebserver)
    androidTestImplementation(libs.media3.exoplayer)
    debugImplementation(libs.compose.ui.test.manifest)
}
