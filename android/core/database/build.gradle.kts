plugins {
    id("autoanime.android.library")
    id("autoanime.hilt")
}

android { namespace = "com.autoanime.android.core.database" }

dependencies {
    api(libs.room.runtime)
    api(libs.room.ktx)
    ksp(libs.room.compiler)
}
