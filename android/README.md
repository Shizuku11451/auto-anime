# Android client

The client is a multi-module Compose app (`app`, `core/*`, `feature/*`, `benchmark`); shared Gradle setup lives in `build-logic`.

Requires JDK 17 or newer (verified with JDK 21) and Android SDK platform 37 (`platforms;android-37.0`) with Build Tools 37.0.0. The Gradle 9.8.2 wrapper is checked in and checksum-pinned; dependency and plugin versions live in `gradle/libs.versions.toml`.

From this directory:

```sh
./gradlew testDebugUnitTest assembleDebug lintDebug
```

For the isolated benchmark test APK build, run `./gradlew :benchmark:assembleBenchmark`. See `../docs/ANDROID_IMPLEMENTATION.md` for recorded verification and remaining device acceptance; benchmark compilation is not a performance result.

On Windows, use `gradlew.bat` with the same tasks. The APK is written to `app/build/outputs/apk/debug/app-debug.apk`. Android SDK setup is local (`local.properties` is ignored); do not commit SDK paths or signing credentials. The first build requires network access to download the pinned Gradle distribution and declared Maven dependencies.

The project source should be checked out at an ASCII-only path on Windows if Gradle/JDK rejects a Unicode path. `android.overridePathCheck=true` is enabled as a compatibility workaround; this does not replace a clean checkout build. Device install and acceptance are separate from compilation and unit tests.
