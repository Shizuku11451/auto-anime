plugins {
    id("autoanime.android.library")
    alias(libs.plugins.kotlin.serialization)
}

android { namespace = "com.autoanime.android.core.network" }

dependencies {
    api(projects.core.model)
    api(libs.okhttp)
    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.kotlinx.serialization.json)
    testImplementation(libs.okhttp.mockwebserver)
    androidTestImplementation(libs.androidx.test.ext.junit)
    androidTestImplementation(libs.androidx.test.runner)
}
