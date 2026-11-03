plugins {
    id("autoanime.android.library")
    id("autoanime.android.compose")
}

android { namespace = "com.autoanime.android.core.designsystem" }

dependencies {
    api(projects.core.model)
    api(libs.compose.material3)
    api(libs.compose.foundation)
    api(libs.coil.compose)
}
