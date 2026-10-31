# Android client implementation status

The M0/M1 Android client prototype is delivered for emulator evaluation; M2-M4 remain out of scope. Physical-device acceptance is deferred.

## Implementation

- Kotlin/XML Android Views app, API 26+, compile SDK 36, target SDK 35, Gradle 8.13 wrapper with pinned distribution checksum.
- HTTPS by default; HTTP requires explicit user confirmation. Requests preserve a reverse-proxy prefix, reject cross-origin redirects, and scope encrypted Keystore AES-GCM session cookies by origin and account. Passwords are not persisted.
- Setup/login/logout, bounded library pagination, detail, covers and saved-item Media3 streaming are implemented. A server missing the bounded page response fails with an actionable server-update message; no full-array fallback is used. Retrofit is deliberately not used: the API is a small custom OkHttp coroutine client, so unused Retrofit/converter dependencies were removed.
- Macrobenchmark module uses AGP experimental self-instrumenting mode (`android.experimental.self-instrumenting=true`), runs the target in the test APK process, and keeps the app's benchmark build minified/profileable. Startup and library-scroll scenarios exist; scroll requires fixed demo server data.
- Debug builds enable logged StrictMode thread/VM diagnostics only. `MainActivity` calls `reportFullyDrawn()` on the first pre-draw of the navigation host; this is a UI-readiness marker and does not wait for connection restore, library data, or covers. It is not a separately validated fully-interactive benchmark metric; benchmark scenarios must measure that separately from first frame and network/data/cover completion.

## Verification evidence

The latest full clean Android verification (from an ASCII-path junction, Gradle 8.13/JDK 21, in-process Kotlin compiler) ran:

```text
gradle.bat --no-daemon -Pkotlin.compiler.execution.strategy=in-process clean testDebugUnitTest assembleDebug assembleDebugAndroidTest lintDebug assembleBenchmark :benchmark:assembleBenchmark
BUILD SUCCESSFUL
```

The latest complete Gradle verification passed (161 tasks, 10 executed, 151 up-to-date; not a clean build): 31 unit tests, app debug and instrumentation APKs, lint, app benchmark and benchmark test APKs. Lint reported 0 errors and 31 warnings. Dedicated disposable `emulator-5580` instrumentation passed all 9 tests (16.852 seconds). The synthetic playback case verified first-frame rendering, the Android client's HEAD/Range request contract and unauthenticated 401, seek position/discontinuity, and Close navigation/player release. This does not validate the complete Go stream endpoint or unbuffered seek behavior. The fixture is four-second 320x180 H.264 Baseline/AAC generated from FFmpeg `lavfi` patterns and sine audio; synthetic, no external media or product dependency. SHA-256: `0ab403be8022af247006ba4c1d8ea9967e25f291cc298f79662189f9b3903e35`. Generator: `android/scripts/generate-playback-fixture.ps1`; requires local FFmpeg with `libx264` and AAC support; FFmpeg and its LGPL/GPL configuration are not bundled. Tests require disposable app data without a saved connection; cleanup only clears the synthetic account/origin test cookie and connection after that guard passes.

A separate 30-iteration cold-start connect-screen Macrobenchmark passed on the emulator (`OK (1 test)`): time-to-full-display median 538.81 ms, P95 673.16 ms; frame-overrun P95 386.55 ms. These are emulator-only observations captured before the final WindowInsets change, not a final baseline or performance acceptance. Trace output: `C:/aa-tmp/startup30-fixed-instrumentation.log`; the JSON metrics are in the accompanying `startup30-traces` output. No physical-device tests or codec matrix are claimed.

## Dependency licenses

The resolved debug runtime dependency inventory (127 artifacts) and each artifact's published POM license/source metadata are recorded in [ANDROID_DEPENDENCIES.md](ANDROID_DEPENDENCIES.md). The inventory is not an SPDX/SBOM or a legal review, and excludes test-only and benchmark-only dependencies. Confirm notices and applicable distribution terms before release; Android SDK/Gradle tooling has separate terms.

## Acceptance still required

Still required: physical-device acceptance, codec/subtitle combinations, accessibility review, and startup/scroll performance evaluation using fixed isolated server data. Emulator UI tests do not establish hardware decoding, device compatibility, or performance acceptance. No NAS acceptance is implied.
