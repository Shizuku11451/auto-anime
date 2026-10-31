# Android client implementation status

This prototype covers the M0/M1 scope in `ANDROID_PLAN.md`; M2-M4 are out of scope and M1 is not accepted until device checks pass.

## Implementation

- Kotlin/XML Android Views app, API 26+, compile SDK 36, target SDK 35, Gradle 8.13 wrapper with pinned distribution checksum.
- HTTPS by default; HTTP requires explicit user confirmation. Requests preserve a reverse-proxy prefix, reject cross-origin redirects, and scope encrypted Keystore AES-GCM session cookies by origin and account. Passwords are not persisted.
- Setup/login/logout, bounded library pagination, detail, covers and saved-item Media3 streaming are implemented. A server missing the bounded page response fails with an actionable server-update message; no full-array fallback is used. Retrofit is deliberately not used: the API is a small custom OkHttp coroutine client, so unused Retrofit/converter dependencies were removed.
- Macrobenchmark module isolates the app under `com.autoanime.android.benchmark`, uses the dedicated minified/profileable build type and debug test signing. Startup and library-scroll scenarios are scaffolds, not measured results. Scroll requires fixed demo server data.
- Debug builds enable logged StrictMode thread/VM diagnostics only. `MainActivity` calls `reportFullyDrawn()` on the first pre-draw of the navigation host; this is a UI-readiness marker and does not wait for connection restore, library data, or covers. It is not a separately validated fully-interactive benchmark metric; benchmark scenarios must measure that separately from first frame and network/data/cover completion.

## Verification evidence

The latest full clean Android verification (from an ASCII-path junction, Gradle 8.13/JDK 21, in-process Kotlin compiler) ran:

```text
gradle.bat --no-daemon -Pkotlin.compiler.execution.strategy=in-process clean testDebugUnitTest assembleDebug assembleDebugAndroidTest lintDebug assembleBenchmark :benchmark:assembleBenchmark
BUILD SUCCESSFUL
```

All requested tasks passed after `clean` (160 tasks, 134 executed): 29 unit tests (15 API, 8 connection, 5 ViewModel and 1 playback-error test), app debug build, Android instrumentation APK build, lint, app benchmark build and benchmark test APK build. Lint reported 0 errors and 30 warnings. The instrumentation suite covering navigation graph back-stack behavior and encrypted cookie-storage behavior compiled but was not run; no connected device was available (`adb` reported no devices). Both benchmark APKs are debug-signed; metadata identifies target `com.autoanime.android.benchmark` and instrumentation `com.autoanime.android.benchmark.test`. No physical-device tests or performance measurements have been collected.

## Dependency licenses

The resolved debug runtime dependency inventory (127 artifacts) and each artifact's published POM license/source metadata are recorded in [ANDROID_DEPENDENCIES.md](ANDROID_DEPENDENCIES.md). The inventory is not an SPDX/SBOM or a legal review, and excludes test-only and benchmark-only dependencies. Confirm notices and applicable distribution terms before release; Android SDK/Gradle tooling has separate terms.

## Acceptance still required

Still required: run the compiled Android instrumentation suite on a device; install on supported physical Android hardware; verify real cookies, covers, stream authentication/range behavior, codecs/subtitles, lifecycle, accessibility, and startup/scroll metrics using isolated temporary server data and legal sample media. No NAS or real-device acceptance is implied by compilation.
