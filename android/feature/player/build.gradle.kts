plugins {
    id("autoanime.android.feature")
}

android { namespace = "com.autoanime.android.feature.player" }

dependencies {
    implementation(libs.media3.exoplayer)
    implementation(libs.media3.datasource.okhttp)
    implementation(libs.media3.ui.compose.material3)
}
