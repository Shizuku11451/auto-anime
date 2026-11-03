plugins {
    id("autoanime.android.library")
    id("autoanime.hilt")
}

android { namespace = "com.autoanime.android.core.data" }

dependencies {
    api(projects.core.model)
    api(projects.core.network)
    api(libs.kotlinx.coroutines.android)
    implementation(libs.androidx.datastore.preferences)
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.coil.network.okhttp)
    api(libs.coil.compose)
    testImplementation(libs.turbine)
}
