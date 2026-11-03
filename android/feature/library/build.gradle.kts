plugins {
    id("autoanime.android.feature")
}

android { namespace = "com.autoanime.android.feature.library" }

dependencies {
    implementation(libs.compose.material3.adaptive)
}
