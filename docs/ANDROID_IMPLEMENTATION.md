# Android client implementation status

This prototype covers the M0/M1 scope in `ANDROID_PLAN.md`; M2-M4 are out of scope and M1 is not accepted until device checks pass.

## Implementation

- Kotlin/XML Android Views app, API 26+, compile SDK 36, target SDK 35, Gradle 8.13 wrapper with pinned distribution checksum.
- HTTPS by default; HTTP requires explicit user confirmation. Requests preserve a reverse-proxy prefix, reject cross-origin redirects, and scope encrypted Keystore AES-GCM session cookies by origin and account. Passwords are not persisted.
- Setup/login/logout, bounded library pagination, detail, covers and saved-item Media3 streaming are implemented. A server missing the bounded page response fails with an actionable server-update message; no full-array fallback is used. Retrofit is deliberately not used: the API is a small custom OkHttp coroutine client, so unused Retrofit/converter dependencies were removed.
- Macrobenchmark module isolates the app under `com.autoanime.android.benchmark`, uses the dedicated minified/profileable build type and debug test signing. Startup and library-scroll scenarios are scaffolds, not measured results. Scroll requires fixed demo server data.

## Verification evidence

The last recorded successful Android command (from an ASCII-path junction, Gradle 8.13/JDK 21, in-process Kotlin compiler) was:

```text
gradle.bat --no-daemon -Pkotlin.compiler.execution.strategy=in-process testDebugUnitTest assembleDebug lintDebug assembleBenchmark :benchmark:assembleBenchmark
BUILD SUCCESSFUL
```

The latest complete Gradle run passed 29 unit tests (15 API, 8 connection, 5 ViewModel and 1 playback-error test), app debug assemble, lint, app benchmark assemble, and benchmark test APK assemble; lint reported 28 warnings and 0 errors. Both benchmark APKs are debug-signed; metadata identifies target `com.autoanime.android.benchmark` and instrumentation `com.autoanime.android.benchmark.test`. `assembleDebugAndroidTest` also successfully compiled the navigation/back-stack and encrypted cookie-storage instrumentation sources (origin-bound ciphertext, fail-closed corruption and immediate-save behavior); no instrumentation was executed. No physical-device tests or performance measurements have been collected.

## Dependency licenses

Direct dependencies are AndroidX (Apache-2.0), Kotlin/Kotlinx (Apache-2.0), OkHttp and MockWebServer (Apache-2.0), Coil (Apache-2.0), and Media3 (Apache-2.0); JUnit is EPL-1.0. Android SDK/Gradle tooling has separate terms. Confirm the resolved transitive license inventory and notices before distribution; this summary is not a generated SBOM.

## Acceptance still required

Still required: run the compiled Android instrumentation suite on a device, install on supported physical Android hardware, verify real cookies, covers, stream authentication/range behavior, codecs/subtitles, lifecycle, accessibility, and startup/scroll metrics using isolated temporary server data and legal sample media. Confirm resolved transitive dependency licenses and notices before distribution; the direct-license list above is not an SBOM. No NAS or real-device acceptance is implied by compilation.
