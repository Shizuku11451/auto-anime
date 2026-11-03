plugins {
    id("autoanime.android.feature")
}

android { namespace = "com.autoanime.android.feature.connect" }

dependencies {
    implementation(libs.androidx.activity.compose)
}
