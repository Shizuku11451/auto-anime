pluginManagement {
    includeBuild("build-logic")
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}
dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}
rootProject.name = "AutoAnimeAndroid"
include(
    ":app",
    ":benchmark",
    ":core:model",
    ":core:network",
    ":core:data",
    ":core:designsystem",
    ":feature:connect",
    ":feature:discover",
    ":feature:downloads",
    ":feature:feeds",
    ":feature:library",
    ":feature:player",
    ":feature:settings",
)
enableFeaturePreview("TYPESAFE_PROJECT_ACCESSORS")
