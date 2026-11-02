# Android client implementation status

The M0/M1 Android client prototype is delivered for emulator evaluation; M2-M4 remain out of scope. Physical-device function and performance acceptance remain pending.

## Implementation

- Kotlin/XML Android Views app, API 26+, compile SDK 36, target SDK 35, Gradle 8.13 wrapper with pinned distribution checksum.
- HTTPS by default; HTTP requires explicit user confirmation. Requests preserve a reverse-proxy prefix, reject cross-origin redirects, and scope encrypted Keystore AES-GCM session cookies by origin and account. Passwords are not persisted.
- Setup/login/logout, bounded library pagination, detail, covers and saved-item Media3 streaming are implemented. A server missing the bounded page response fails with an actionable server-update message; no full-array fallback is used. Retrofit is deliberately not used: the API is a small custom OkHttp coroutine client, so unused Retrofit/converter dependencies were removed.
- Macrobenchmark module uses AGP experimental self-instrumenting mode (`android.experimental.self-instrumenting=true`), runs the target in the test APK process, and keeps the app's benchmark build minified/profileable. Startup and library-scroll scenarios exist; scroll requires fixed demo server data.
- Debug builds enable logged StrictMode thread/VM diagnostics only. `MainActivity` calls `reportFullyDrawn()` on the first pre-draw of the navigation host; this is a UI-readiness marker and does not wait for connection restore, library data, or covers. It is not a separately validated fully-interactive benchmark metric; benchmark scenarios must measure that separately from first frame and network/data/cover completion.

## Verification evidence

The latest full Android verification (from an ASCII-path junction, Gradle 8.13/JDK 21, in-process Kotlin compiler) ran:

```text
gradle.bat --no-daemon -Pkotlin.compiler.execution.strategy=in-process testDebugUnitTest assembleDebug assembleDebugAndroidTest lintDebug assembleBenchmark :benchmark:assembleBenchmark
BUILD SUCCESSFUL
```

The latest complete Gradle verification after the instrumentation wait fix passed (161 tasks, 7 executed, 154 up-to-date; 41 seconds, not a clean build): 31 unit tests, app debug and instrumentation APKs, lint, app benchmark and benchmark test APKs. Lint reported 0 errors and 31 warnings. Dedicated disposable `emulator-5580` instrumentation passed all 9 tests. Before the wait fix, all 9 physical instrumentation tests passed twice on `cb0456e7` (Xiaomi 2211133C/fuxi, Android 17/API 37, arm64-v8a, 120 Hz; 11.651 and 11.638 seconds), including first-frame rendering, the Android client's HEAD/Range request contract and unauthenticated 401, seek position/discontinuity, and Close navigation/player release. The first physical run was 8/9: the library view appeared in Espresso's failure hierarchy, but `awaitDisplayed()` exited early on `NoMatchingViewException`; this was a test-helper race, not evidence of a product network failure. Commit `47c5727` fixes the helper to retry that exception within its existing deadline. After that fix, installation succeeded, but the final real-device rerun was stopped because the phone had locked and Espresso reported `NoActivityResumedException`; no post-fix physical functional result is claimed. The synthetic playback case does not validate the complete Go stream endpoint or unbuffered seek behavior. The fixture is four-second 320x180 H.264 Baseline/AAC generated from FFmpeg `lavfi` patterns and sine audio; synthetic, no external media or product dependency. SHA-256: `0ab403be8022af247006ba4c1d8ea9967e25f291cc298f79662189f9b3903e35`. Generator: `android/scripts/generate-playback-fixture.ps1`; requires local FFmpeg with `libx264` and AAC support; FFmpeg and its LGPL/GPL configuration are not bundled. Tests require disposable app data without a saved connection; cleanup only clears the synthetic account/origin test cookie and connection after that guard passes.

A separate 30-iteration cold-start connect-screen Macrobenchmark passed on the emulator after the final WindowInsets change (`OK (1 test)`, 73.885 seconds): initial/full-display time median 514.895 ms and P95 578.376 ms (nearest-rank, 29th sorted sample (zero-based index 28)); frame-overrun P95 378.675 ms. These are emulator-only observations, not a final performance baseline or acceptance. JSON metrics: `C:/aa-tmp/startup30-final.json`. A physical-device 30-iteration run produced zero valid samples and no JSON: logcat showed `dex2oat32` missing; read-only checks found an arm64-only device with `dex2oat64` present and a `zygote64_32` configuration. This suggests an ART/runtime environment limitation but does not establish the full cause. No build-mode changes or system modifications were made. No physical-device performance result or codec matrix is claimed.

## Dependency licenses

The resolved debug runtime dependency inventory (127 artifacts) and each artifact's published POM license/source metadata are recorded in [ANDROID_DEPENDENCIES.md](ANDROID_DEPENDENCIES.md). The inventory is not an SPDX/SBOM or a legal review, and excludes test-only and benchmark-only dependencies. Confirm notices and applicable distribution terms before release; Android SDK/Gradle tooling has separate terms.

## Acceptance still required

Still required: rerun post-fix physical instrumentation while the device is unlocked and an activity is resumed; investigate whether the phone's custom ART environment can support Macrobenchmark without altering system settings or build mode; codec/subtitle combinations; accessibility review; and startup/scroll evaluation using fixed isolated server data. Emulator UI tests do not establish hardware decoding, device compatibility, or performance acceptance. No NAS acceptance is implied.
